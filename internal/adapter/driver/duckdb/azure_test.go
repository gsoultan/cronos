//go:build duckdb

package duckdb

import (
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

func blob(creds string) definition.DataSource {
	return definition.DataSource{
		Name: "events", Driver: "object-store",
		URI: "az://lake/events/", Format: "parquet", Credentials: creds,
	}
}

/*
TestAnAzureSourceCarriesAConnectionString covers the shape Azure wants.

Its secret takes a connection string or an account name and refuses
ACCOUNT_KEY as a parameter — the key belongs inside the string. A connection
string is also semicolons and equals signs, which is what the credentials
format separates fields with, so the definition names the parts and the mount
assembles them.
*/
func TestAnAzureSourceCarriesAConnectionString(t *testing.T) {
	stmts, err := mount("events", blob("account_name=acme;account_key=AZKEYVALUE=="))
	if err != nil {
		t.Fatal(err)
	}
	got := sqlOf(stmts)
	for _, want := range []string{
		"INSTALL azure", "LOAD azure",
		"CREATE OR REPLACE SECRET events_store (TYPE azure, CONNECTION_STRING",
		"AccountName=acme", "AccountKey=AZKEYVALUE==",
		"EndpointSuffix=core.windows.net",
		"SCOPE 'az://lake/events/'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "httpfs") {
		t.Errorf("an Azure source loaded httpfs:\n%s", got)
	}

	// The key is inside the connection string, so the string is what has to be
	// strippable from an error — the key alone would leave the rest of it,
	// which is most of a credential.
	var declared bool
	for _, s := range stmts {
		for _, h := range s.holds {
			if strings.Contains(h, "AZKEYVALUE==") {
				declared = true
			}
		}
	}
	if !declared {
		t.Error("the account key is in the sql and not in holds")
	}
}

// All three spellings reach Azure, because three tools emit three of them.
func TestEverySpellingOfAzureIsAzure(t *testing.T) {
	for _, scheme := range []string{"az", "azure", "abfss"} {
		src := blob("account_name=acme;account_key=k")
		src.URI = scheme + "://lake/events/"
		stmts, err := mount("events", src)
		if err != nil {
			t.Fatalf("%s://: %v", scheme, err)
		}
		if got := sqlOf(stmts); !strings.Contains(got, "TYPE azure") {
			t.Errorf("%s:// did not become an Azure secret:\n%s", scheme, got)
		}
	}
}

// An endpoint is part of Azure's connection string rather than a parameter
// beside it, which is what makes an emulator or a private cloud reachable.
func TestAnAzureEndpointGoesInsideTheConnectionString(t *testing.T) {
	src := blob("account_name=devstoreaccount1;account_key=k")
	src.Endpoint = "http://127.0.0.1:10000"
	stmts, err := mount("events", src)
	if err != nil {
		t.Fatal(err)
	}
	got := sqlOf(stmts)
	for _, want := range []string{
		"DefaultEndpointsProtocol=http;",
		"BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "URL_STYLE") {
		t.Errorf("an Azure secret got S3's endpoint parameters:\n%s", got)
	}
}

// The managed-identity path. A chain answers who the caller is and not which
// account, so the account name stays.
func TestAnAzureChainKeepsTheAccountName(t *testing.T) {
	stmts, err := mount("events", blob("provider=chain;account_name=acme"))
	if err != nil {
		t.Fatal(err)
	}
	got := sqlOf(stmts)
	for _, want := range []string{"PROVIDER credential_chain", "ACCOUNT_NAME 'acme'"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "CONNECTION_STRING") {
		t.Errorf("a chain invented a connection string:\n%s", got)
	}
}

/*
TestCredentialsFromTheWrongCloudAreNamed is the same rule as a credential on a
local path: never silently ignored.

Passing an Azure key to an S3 secret builds a statement with a parameter the
store has never heard of, and passing an S3 key to Azure builds one with no
account at all. Both fail either way; the difference is whether the message
describes the mistake or reports the database's confusion about it.
*/
func TestCredentialsFromTheWrongCloudAreNamed(t *testing.T) {
	lake := definition.DataSource{
		Name: "events", Driver: "object-store",
		URI: "s3://acme-lake/events/", Format: "parquet",
		Credentials: "account_name=acme;account_key=SECRETKEYVALUE",
	}
	_, err := mount("events", lake)
	if err == nil {
		t.Fatal("an Azure credential on an S3 bucket was accepted")
	}
	if !strings.Contains(err.Error(), "account_name") {
		t.Errorf("the message does not name the field: %v", err)
	}
	if strings.Contains(err.Error(), "SECRETKEYVALUE") {
		t.Errorf("the refusal printed the key: %v", err)
	}

	_, err = mount("events", blob("key_id=AKIA;secret=SECRETKEYVALUE"))
	if err == nil {
		t.Fatal("an S3 credential on an Azure container was accepted")
	}
	if !strings.Contains(err.Error(), "account_name") {
		t.Errorf("the message does not say what is missing: %v", err)
	}
	if strings.Contains(err.Error(), "SECRETKEYVALUE") {
		t.Errorf("the refusal printed the key: %v", err)
	}
}

// Azure resolves a region from the account name and its blob endpoints take
// none, so a region here would be a setting that does nothing.
func TestAzureRefusesARegion(t *testing.T) {
	src := blob("account_name=acme;account_key=k")
	src.Region = "westeurope"
	if _, err := mount("events", src); err == nil {
		t.Fatal("a region on an Azure source was accepted")
	}
}
