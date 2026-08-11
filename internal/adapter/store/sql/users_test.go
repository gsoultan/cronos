package sql_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
)

func dewi() identity.User {
	return identity.User{ID: "u1", Email: "dewi@acme.example", Name: "Dewi",
		Org: "acme", Project: "finance", Role: "editor"}
}

const secret = "correct horse battery staple"

func TestSigningIn(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	if err := s.CreateUser(ctx, dewi(), secret); err != nil {
		t.Fatal(err)
	}

	got, err := s.Authenticate(ctx, "dewi@acme.example", secret)
	if err != nil {
		t.Fatalf("the right password was refused: %v", err)
	}
	if got.Role != "editor" || got.Project != "finance" {
		t.Errorf("user = %+v", got)
	}
	if _, err := s.Authenticate(ctx, "dewi@acme.example", "wrong"); !errors.Is(err, identity.ErrBadCredentials) {
		t.Errorf("a wrong password was accepted: %v", err)
	}
}

// Somebody typing Dewi@Acme.example at six in the morning is the same person.
func TestEmailsAreCaseAndSpaceInsensitive(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if err := s.CreateUser(ctx, dewi(), secret); err != nil {
		t.Fatal(err)
	}

	for _, typed := range []string{"Dewi@Acme.example", "  dewi@acme.example  ", "DEWI@ACME.EXAMPLE"} {
		if _, err := s.Authenticate(ctx, typed, secret); err != nil {
			t.Errorf("%q was refused: %v", typed, err)
		}
	}
	// And they cannot register a second account under the same address.
	second := dewi()
	second.ID, second.Email = "u2", "DEWI@acme.example"
	if err := s.CreateUser(ctx, second, secret); !errors.Is(err, identity.ErrExists) {
		t.Errorf("a duplicate registered: %v", err)
	}
}

// Telling "no such user" apart from "wrong password" is how somebody learns
// which addresses have accounts — worth more to a phisher than to anyone else.
func TestAnUnknownEmailIsIndistinguishableFromAWrongPassword(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if err := s.CreateUser(ctx, dewi(), secret); err != nil {
		t.Fatal(err)
	}

	_, unknown := s.Authenticate(ctx, "nobody@acme.example", secret)
	_, wrong := s.Authenticate(ctx, "dewi@acme.example", "not the password")

	if unknown == nil || wrong == nil {
		t.Fatal("both should fail")
	}
	if unknown.Error() != wrong.Error() {
		t.Errorf("the two are distinguishable:\n  %v\n  %v", unknown, wrong)
	}
}

// And they must not be distinguishable by how long they take, either: an
// unknown address answering in a millisecond while a known one takes eighty is
// the same enumeration attack with a stopwatch.
func TestAnUnknownEmailCostsTheSameAsAKnownOne(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if err := s.CreateUser(ctx, dewi(), secret); err != nil {
		t.Fatal(err)
	}

	known := elapsed(func() { s.Authenticate(ctx, "dewi@acme.example", "wrong") })
	unknown := elapsed(func() { s.Authenticate(ctx, "nobody@acme.example", "wrong") })

	// Within an order of magnitude. Timing on a shared runner is noisy, and
	// the failure this guards against is a thousandfold difference — the one
	// that happens when an unknown email returns before hashing anything.
	if unknown*10 < known {
		t.Errorf("unknown took %s, known took %s — an absent user answers too fast",
			unknown, known)
	}
}

func elapsed(f func()) time.Duration {
	began := time.Now()
	f()
	return time.Since(began)
}

func TestAWeakPasswordIsRefusedAtRegistration(t *testing.T) {
	s := open(t)
	if err := s.CreateUser(context.Background(), dewi(), "short"); !errors.Is(err, identity.ErrWeakPassword) {
		t.Errorf("got %v, want ErrWeakPassword", err)
	}
}

// A struct holding a hash gets logged, serialised into an error, and returned
// from an API by somebody adding a field.
func TestAUserNeverCarriesItsHash(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if err := s.CreateUser(ctx, dewi(), secret); err != nil {
		t.Fatal(err)
	}

	got, err := s.User(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), "$2a$") {
		t.Error("a bcrypt hash is in the user struct")
	}
}
