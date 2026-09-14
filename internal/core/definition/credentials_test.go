package definition

import (
	"errors"
	"strings"
	"testing"
)

func TestParseCredentials(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want map[string]string
	}{
		{"a key and a secret", "key_id=AKIA;secret=wJal",
			map[string]string{"key_id": "AKIA", "secret": "wJal"}},
		{"spaces around the pairs", " key_id = AKIA ; secret = wJal ",
			map[string]string{"key_id": "AKIA", "secret": "wJal"}},
		{"newline separated, which is what a file holds", "key_id=AKIA\nsecret=wJal",
			map[string]string{"key_id": "AKIA", "secret": "wJal"}},
		{"a session token", "key_id=A;secret=B;session_token=C",
			map[string]string{"key_id": "A", "secret": "B", "session_token": "C"}},
		{"r2 addresses by account", "key_id=A;secret=B;account_id=acct",
			map[string]string{"key_id": "A", "secret": "B", "account_id": "acct"}},
		{"a value holding an equals sign, which base64 does",
			"key_id=A;secret=abc==", map[string]string{"key_id": "A", "secret": "abc=="}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseCredentials(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("%s: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// A credential that cannot authenticate anything is refused at the edit that
// introduced it, because the alternative is a 403 from the object store months
// later that reads like the key was rotated.
func TestCredentialsThatCannotWork(t *testing.T) {
	for _, c := range []struct{ name, in string }{
		{"a typo in a key name", "key_id=A;secret_key=B"},
		{"a field with no value at all", "key_id=A;secret"},
		{"a key set to nothing", "key_id=A;secret="},
		{"a secret with no key", "secret=B"},
		{"a key with no secret", "key_id=A"},
		{"nothing but separators", ";;"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseCredentials(c.in)
			if err == nil {
				t.Fatal("accepted")
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("got %v, want ErrInvalid", err)
			}
		})
	}
}

// The message names the key that was not recognised. "credentials are invalid"
// sends somebody to re-read the whole block; the name sends them to the line.
func TestAnUnknownCredentialKeyIsNamed(t *testing.T) {
	_, err := ParseCredentials("key_id=A;secret=B;secrit=C")
	if err == nil {
		t.Fatal("accepted")
	}
	if !strings.Contains(err.Error(), "secrit") {
		t.Errorf("the message does not name the key: %v", err)
	}
}

func lake(creds string) DataSource {
	return DataSource{
		Name: "events-lake", Driver: "object-store",
		URI: "s3://acme-lake/events/", Format: "parquet", Credentials: creds,
	}
}

func TestValidateChecksCredentials(t *testing.T) {
	if err := lake("key_id=A;secret=B").Validate(); err != nil {
		t.Errorf("a working credential was refused: %v", err)
	}
	if err := lake(CredentialChain).Validate(); err != nil {
		t.Errorf("the environment chain was refused: %v", err)
	}
	if err := lake("").Validate(); err != nil {
		t.Errorf("a public bucket was refused: %v", err)
	}
	if err := lake("key_id=A;secret_key=B").Validate(); err == nil {
		t.Error("a typo in a key name was stored")
	}
}

/*
TestAnUnresolvedReferenceIsNotAShapeError is the one that would fail closed.

A definition holds ${secret:lake_creds}, not the pairs — that is the point of a
reference. Parsing it as pairs makes every real definition unsavable, and the
failure would read as a malformed credential rather than as a check running at
the wrong moment.
*/
func TestAnUnresolvedReferenceIsNotAShapeError(t *testing.T) {
	if err := lake("${secret:lake_creds}").Validate(); err != nil {
		t.Fatalf("a definition using a secret reference could not be saved: %v", err)
	}
}

// An endpoint decides where a request goes and whether it is encrypted. A host
// with no scheme leaves the second to whoever reads the file.
func TestValidateChecksTheEndpointScheme(t *testing.T) {
	with := func(endpoint string) DataSource {
		d := lake("key_id=A;secret=B")
		d.Endpoint = endpoint
		return d
	}
	for _, ok := range []string{"", "http://minio.internal:9000", "https://s3.acme.internal"} {
		if err := with(ok).Validate(); err != nil {
			t.Errorf("endpoint %q was refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"minio.internal:9000", "s3://minio.internal", "//minio.internal"} {
		if err := with(bad).Validate(); err == nil {
			t.Errorf("endpoint %q was stored without a scheme", bad)
		}
	}
	// Still a reference at save time, so its shape is not knowable yet.
	if err := with("${secret:lake_endpoint}").Validate(); err != nil {
		t.Errorf("an endpoint held in a secret could not be saved: %v", err)
	}
}
