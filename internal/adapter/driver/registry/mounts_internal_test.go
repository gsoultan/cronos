package registry

import (
	"io"
	"log/slog"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/platform/secret"

	_ "modernc.org/sqlite"
)

/*
A federation attaches each database with its password in place, not with the
reference to it.

open resolved the DSN into the pool it built and kept the definition it was
given, and a federation mounts from the definition — so DuckDB was asked to
ATTACH the literal text ${secret:warehouse_pw}. Every source that took its
password from a secret, which is every source connected through the portal,
opened for a single-database report and failed the moment a dataset joined it
to anything else. Object stores never had this: their URI and credentials were
resolved into the definition, which is why only databases broke.
*/
func TestAFederationMountsTheResolvedDSN(t *testing.T) {
	r, err := New([]definition.DataSource{{
		Name: "warehouse", Driver: "sqlite",
		DSN: "file:${secret:warehouse_file}?mode=memory&cache=shared",
	}}, secret.Map{"warehouse_file": "federated"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })

	mounts, _, err := r.mounts(definition.Dataset{
		Name: "joined", Sources: []definition.SourceRef{{Ref: "warehouse"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := mounts["warehouse"].DSN; got != "file:federated?mode=memory&cache=shared" {
		t.Errorf("the federation would attach %q", got)
	}
}
