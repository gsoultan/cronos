//go:build duckdb

package duckdb

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
Against a real object store.

Everything else in this package asserts the SQL a credential becomes. That
proves the statement is the one intended and nothing about whether the store
accepts it — which is the only question a credential exists to answer, and one
that cannot be asked without a server that authenticates.

scripts/live-objectstore.sh brings one up and sets the variables below. Without
them these skip, so the ordinary suite stays hermetic.
*/
func liveStore(t *testing.T) (endpoint, key, secret, uri string) {
	t.Helper()
	endpoint = os.Getenv("CRONOS_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set CRONOS_S3_ENDPOINT — see scripts/live-objectstore.sh")
	}
	return endpoint, os.Getenv("CRONOS_S3_KEY"), os.Getenv("CRONOS_S3_SECRET"),
		os.Getenv("CRONOS_S3_URI")
}

func lakeSource(name, uri, endpoint, creds string) definition.DataSource {
	return definition.DataSource{
		Name: name, Driver: "object-store", URI: uri, Format: "parquet",
		Region: "us-east-1", Endpoint: endpoint, Credentials: creds,
	}
}

// sum reads the dataset over the given mounts and returns the total.
func sum(t *testing.T, sources map[string]definition.DataSource, ds definition.Dataset) (float64, error) {
	t.Helper()
	f, err := Open(context.Background(), sources)
	if err != nil {
		return 0, err
	}
	t.Cleanup(func() { _ = f.Close() })

	plan, err := query.NewBuilder(query.Postgres{}).Build(ds, nil, principal.Principal{})
	if err != nil {
		t.Fatalf("compiling: %v", err)
	}
	rows, err := f.Executor().Execute(context.Background(), plan)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	total := 0.0
	for rows.Next() {
		var region string
		var amount float64
		if err := rows.Scan(&region, &amount); err != nil {
			return 0, err
		}
		total += amount
	}
	return total, rows.Err()
}

// TestALiveObjectStoreAuthenticatesAndReads is the whole point: a key from a
// definition, a server that checks it, and rows on the other side.
func TestALiveObjectStoreAuthenticatesAndReads(t *testing.T) {
	endpoint, key, secret, uri := liveStore(t)

	got, err := sum(t, map[string]definition.DataSource{
		"events": lakeSource("events", uri, endpoint, "key_id="+key+";secret="+secret),
	}, dataset())
	if err != nil {
		t.Fatalf("a private bucket with the right key would not read: %v", err)
	}
	if got != 125 {
		t.Errorf("summed to %.2f, the lake holds 125", got)
	}
}

/*
TestALiveObjectStoreRefusesTheWrongKey is the half that proves the first one
means something.

A read that works tells you nothing on its own: a bucket left open to the world
reads exactly the same way. What says the credential is load-bearing is that
changing it stops the read.

It also checks what comes back. The secret is in the statement that created it
and DuckDB quotes a failing statement into its error, so a refusal is the most
likely moment for a key to reach a log.
*/
func TestALiveObjectStoreRefusesTheWrongKey(t *testing.T) {
	endpoint, key, secret, uri := liveStore(t)

	for _, c := range []struct{ name, creds string }{
		{"a wrong secret", "key_id=" + key + ";secret=" + secret + "-wrong"},
		{"a key nobody issued", "key_id=nobody-at-all;secret=" + secret},
		{"no credentials at all", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := sum(t, map[string]definition.DataSource{
				"events": lakeSource("events", uri, endpoint, c.creds),
			}, dataset())
			if err == nil {
				t.Fatal("the store served a private bucket to it")
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("the refusal carries the real secret:\n%v", err)
			}
		})
	}
}

/*
TestTwoLiveLakesAuthenticateAsThemselves is what SCOPE is for.

A DuckDB secret with no scope is the default for every bucket on the
connection, so two lakes with a key each would both resolve to whichever was
created last — and the failure is not a refusal but the wrong identity, which
on a store that happens to allow both reads nobody would notice.

The two credentials here can each read one bucket and not the other, so
last-wins fails and correct scoping is the only way both sides return rows.
*/
func TestTwoLiveLakesAuthenticateAsThemselves(t *testing.T) {
	endpoint, _, _, uriA := liveStore(t)
	keyA, secretA := os.Getenv("CRONOS_S3_KEY_A"), os.Getenv("CRONOS_S3_SECRET_A")
	keyB, secretB := os.Getenv("CRONOS_S3_KEY_B"), os.Getenv("CRONOS_S3_SECRET_B")
	uriB := os.Getenv("CRONOS_S3_URI_B")
	if keyA == "" || keyB == "" || uriB == "" {
		t.Skip("set CRONOS_S3_KEY_A/_B and CRONOS_S3_URI_B for the scope check")
	}

	joined := definition.Dataset{
		Name: "both",
		Sources: []definition.SourceRef{
			{Ref: "events"}, {Ref: "other"},
		},
		Query: "SELECT region, SUM(amount) AS total FROM (" +
			"SELECT region, amount FROM events UNION ALL SELECT region, amount FROM other" +
			") GROUP BY region ORDER BY region",
		Fields: []definition.Field{
			{Name: "region", Type: "string", Role: definition.Dimension},
			{Name: "total", Type: "number", Role: definition.Measure},
		},
	}

	got, err := sum(t, map[string]definition.DataSource{
		"events": lakeSource("events", uriA, endpoint, "key_id="+keyA+";secret="+secretA),
		"other":  lakeSource("other", uriB, endpoint, "key_id="+keyB+";secret="+secretB),
	}, joined)
	if err != nil {
		t.Fatalf("two lakes with a key each did not both read — the scope on one "+
			"secret is not holding: %v", err)
	}
	if got != 132 {
		t.Errorf("summed to %.2f of 132 — one lake was read and the other was not", got)
	}
}
