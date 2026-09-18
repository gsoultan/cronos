//go:build duckdb

package duckdb

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
Against a real Azure Blob endpoint.

The same argument as objectstore_test.go, for the other cloud and for a
different reason to doubt it: Azure's credential is not a key beside a secret
but a connection string this package assembles, and an assembled string is
exactly the kind of thing that looks right in a unit test and is rejected by
the service. Only a service can say.

scripts/live-azure.sh brings up Azurite and sets the variables below. Without
them these skip, so the ordinary suite stays hermetic.
*/
func liveBlob(t *testing.T) (endpoint, account, key, uri string) {
	t.Helper()
	endpoint = os.Getenv("CRONOS_AZURE_ENDPOINT")
	if endpoint == "" {
		t.Skip("set CRONOS_AZURE_ENDPOINT — see scripts/live-azure.sh")
	}
	return endpoint, os.Getenv("CRONOS_AZURE_ACCOUNT"),
		os.Getenv("CRONOS_AZURE_KEY"), os.Getenv("CRONOS_AZURE_URI")
}

func blobSource(endpoint, account, key, uri string) definition.DataSource {
	return definition.DataSource{
		Name: "events", Driver: "object-store", URI: uri, Format: "parquet",
		Endpoint:    endpoint,
		Credentials: "account_name=" + account + ";account_key=" + key,
	}
}

// TestALiveAzureContainerReads is the whole point: a connection string this
// package built out of two fields is one Azure accepts.
func TestALiveAzureContainerReads(t *testing.T) {
	endpoint, account, key, uri := liveBlob(t)

	total, err := sum(t, map[string]definition.DataSource{
		"events": blobSource(endpoint, account, key, uri),
	}, dataset())
	if err != nil {
		t.Fatalf("a container that exists would not read: %v", err)
	}
	if total != 125 {
		t.Fatalf("summed to %.2f of 125 — the container read, and not all of it", total)
	}
}

/*
TestALiveAzureContainerRefusesAWrongKey is what makes the test above mean
something.

A read that succeeds proves the key was accepted only if a different key would
have been refused. Otherwise it proves the container was open to anybody, and
the credential was decoration.
*/
func TestALiveAzureContainerRefusesAWrongKey(t *testing.T) {
	endpoint, account, key, uri := liveBlob(t)

	// Same length and alphabet, one character different: a key that is wrong
	// rather than malformed, so what refuses it is the service and not a
	// parser on the way there.
	wrong := strings.TrimSuffix(key, "==") + "X="
	if wrong == key {
		t.Skip("could not derive a wrong key from this one")
	}

	_, err := sum(t, map[string]definition.DataSource{
		"events": blobSource(endpoint, account, wrong, uri),
	}, dataset())
	if err == nil {
		t.Fatal("a wrong account key read the container — the key is not being checked")
	}
	// And the error is going to a log.
	for _, leaked := range []string{key, wrong} {
		if strings.Contains(err.Error(), leaked) {
			t.Errorf("the refusal carried an account key:\n%v", err)
		}
	}
}

// An account that does not exist is refused too, and names nothing secret.
func TestALiveAzureContainerRefusesAnUnknownAccount(t *testing.T) {
	endpoint, _, key, uri := liveBlob(t)

	_, err := sum(t, map[string]definition.DataSource{
		"events": blobSource(endpoint, "nosuchaccount", key, uri),
	}, dataset())
	if err == nil {
		t.Fatal("an account nobody created read the container")
	}
	if strings.Contains(err.Error(), key) {
		t.Errorf("the refusal carried the account key:\n%v", err)
	}
}

// The mount is what the service sees. Checked once, live, because every other
// test in this package asserts this string without anything reading it.
func TestTheLiveAzureMountIsWhatWasIntended(t *testing.T) {
	endpoint, account, key, uri := liveBlob(t)

	stmts, err := mount("events", blobSource(endpoint, account, key, uri))
	if err != nil {
		t.Fatal(err)
	}
	got := sqlOf(stmts)
	for _, want := range []string{
		"INSTALL azure", "TYPE azure, CONNECTION_STRING",
		"AccountName=" + account, "BlobEndpoint=" + endpoint + "/" + account,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	_ = context.Background()
}
