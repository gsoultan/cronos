package sql_test

import (
	"context"
	"errors"
	"testing"
	"time"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
Cutting an account's sessions.

There is nothing per-session to delete — a portal token is signed and carries no
record — so this is one timestamp, and everything rests on where it falls and on
whether the standing check can still see it.
*/

func person(t *testing.T, s interface {
	CreateUser(context.Context, identity.User, string) error
}, id string,
) {
	t.Helper()
	if err := s.CreateUser(context.Background(), identity.User{
		ID: id, Email: id + "@acme.example",
		Org: "acme", Project: "finance", Role: "editor",
	}, "a-password-they-chose"); err != nil {
		t.Fatal(err)
	}
}

func TestCuttingSessionsIsVisibleToTheStandingCheck(t *testing.T) {
	both(t, func(t *testing.T, s *store.Store) {
		person(t, s, "usr_ada")

		// Nothing cut yet, which is every account until somebody presses it.
		if _, _, since := s.Active(context.Background(), "usr_ada"); !since.IsZero() {
			t.Fatalf("an untouched account reports a cut at %s", since)
		}

		line, err := s.EndSessions(context.Background(), "usr_ada")
		if err != nil {
			t.Fatal(err)
		}

		known, active, since := s.Active(context.Background(), "usr_ada")
		if !known || !active {
			t.Fatal("cutting sessions disabled the account")
		}
		if !since.Equal(line) {
			t.Fatalf("the check sees %s, the cut was at %s", since, line)
		}
	})
}

/*
The line falls on the next second, not this one.

A token's issue time has second granularity, so a line drawn at 12:00:03.7 and a
session minted at 12:00:03.1 are indistinguishable — and the comparison has to
spare same-second tokens, or the replacement the endpoint mints is refused by
the line it just drew. Rounding up removes the ambiguity rather than narrowing
it, and this is where that happens.
*/
func TestTheCutFallsOnASecondBoundaryAfterNow(t *testing.T) {
	s := open(t)
	person(t, s, "usr_ada")

	line, err := s.EndSessions(context.Background(), "usr_ada")
	if err != nil {
		t.Fatal(err)
	}

	if !line.Equal(line.Truncate(time.Second)) {
		t.Fatalf("the line is not on a second boundary: %s", line)
	}
	// Strictly after the store's clock, so every token minted up to and
	// including this second is on the far side of it.
	now := StoreNow // the test store's clock
	if !line.After(now) {
		t.Fatalf("the line is at %s, the clock says %s", line, now)
	}
}

// Cutting again moves the line rather than adding a row, so pressing the button
// twice is not two answers to the same question.
func TestCuttingTwiceMovesTheLine(t *testing.T) {
	both(t, func(t *testing.T, s *store.Store) {
		person(t, s, "usr_ada")

		first, err := s.EndSessions(context.Background(), "usr_ada")
		if err != nil {
			t.Fatal(err)
		}
		second, err := s.EndSessions(context.Background(), "usr_ada")
		if err != nil {
			t.Fatalf("pressing it twice failed: %v", err)
		}
		if !second.Equal(first) {
			// The test store's clock does not move, so these agree. What matters
			// is that the second one succeeded rather than colliding on the key.
			t.Fatalf("%s then %s", first, second)
		}

		_, _, since := s.Active(context.Background(), "usr_ada")
		if !since.Equal(second) {
			t.Fatalf("the check sees %s after two cuts", since)
		}
	})
}

// A subject that is not an account here is a machine credential. Recording a
// cut for it would be a security event about nobody.
func TestCuttingSessionsForNobodyIsRefused(t *testing.T) {
	s := open(t)

	if _, err := s.EndSessions(context.Background(), "usr_nobody"); !errors.Is(err, identity.ErrNoUser) {
		t.Fatalf("got %v", err)
	}
}

// One account's cut is not another's. They are keyed by user, and a shared row
// would sign out an entire deployment the first time anybody lost a phone.
func TestOneAccountsCutLeavesTheOthersAlone(t *testing.T) {
	s := open(t)
	person(t, s, "usr_ada")
	person(t, s, "usr_grace")

	if _, err := s.EndSessions(context.Background(), "usr_ada"); err != nil {
		t.Fatal(err)
	}

	if _, _, since := s.Active(context.Background(), "usr_grace"); !since.IsZero() {
		t.Fatalf("grace's sessions were cut by ada's: %s", since)
	}
}
