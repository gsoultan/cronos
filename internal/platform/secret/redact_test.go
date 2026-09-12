package secret_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/platform/secret"
)

// A driver quotes back what it was given, so an error assembled without the
// password still arrives holding one.
func TestRedactRemovesTheValue(t *testing.T) {
	const password = "Xq7-warehouse-secret-Zt2"
	err := errors.New(`could not connect: Cannot open file "duckdb://u:` + password + `@h/db"`)

	got := secret.Redact(err, "duckdb://u:"+password+"@h/db")
	if strings.Contains(got.Error(), password) {
		t.Fatalf("the password survived: %v", got)
	}
	if !strings.Contains(got.Error(), "[redacted]") {
		t.Errorf("nothing marks where it was: %v", got)
	}
	if !strings.Contains(got.Error(), "could not connect") {
		t.Errorf("the part somebody needs was lost too: %v", got)
	}
}

// An error with nothing to remove keeps its wrapping, so errors.Is still
// reaches the sentinels a caller matches on.
func TestRedactLeavesAnUnrelatedErrorAlone(t *testing.T) {
	sentinel := errors.New("no credential here")
	wrapped := errors.Join(sentinel, errors.New("and a second reason"))

	if got := secret.Redact(wrapped, "unused", ""); !errors.Is(got, sentinel) {
		t.Errorf("an error with nothing to redact was rewritten: %v", got)
	}
	if secret.Redact(nil, "x") != nil {
		t.Error("nil became an error")
	}
}

// Every occurrence, not the first: a DSN appears twice in a driver error that
// quotes the statement and then points at it.
func TestRedactRemovesEveryOccurrence(t *testing.T) {
	got := secret.Redact(errors.New("bad dsn 'p@ss' near 'p@ss'"), "p@ss")
	if strings.Contains(got.Error(), "p@ss") {
		t.Fatalf("an occurrence survived: %v", got)
	}
}
