package sql_test

import (
	"context"
	"errors"
	"testing"
	"time"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Getting back in, and the four ways that must not become five.

A reset link is the one credential in this product that arrives by email and
authenticates on its own. Everything worth testing here is a "once" that could
quietly become "twice", or a check that could quietly stop being made: a
double-clicked link, a second link still sitting in the mailbox, an account
disabled between asking and clicking, and a session belonging to whoever caused
the reset in the first place.
*/

// asked writes a reset and returns the secret that opens it.
func asked(t *testing.T, s *store.Store, userID, email string, life time.Duration) string {
	t.Helper()

	secret, hash, err := identity.NewReset()
	if err != nil {
		t.Fatal(err)
	}
	r := identity.Reset{
		ID: identity.NewResetID(), UserID: userID, Email: email,
		CreatedAt: StoreNow, Expires: StoreNow.Add(life),
	}
	if err := s.StartReset(context.Background(), r, hash); err != nil {
		t.Fatal(err)
	}
	return secret
}

// account creates one with a password and returns its id. Beside sessions_test's
// `person`, which takes an id and not a password — this needs the password,
// because half of what a reset does is stop the old one working.
func account(t *testing.T, s *store.Store, email, password string) string {
	t.Helper()
	u := identity.User{
		ID: identity.NewID(), Email: email, Name: "Ada",
		Org: "acme", Project: "finance", Role: "admin",
	}
	if err := s.CreateUser(context.Background(), u, password); err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestAResetSetsThePasswordAndSpendsTheLink(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id := account(t, s, "ada@acme.example", "the-old-password-ok")
	secret := asked(t, s, id, "ada@acme.example", time.Hour)

	got, err := s.CompleteReset(ctx, secret, "the-new-password-ok")
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if got.ID != id {
		t.Fatalf("reset returned %s, and the link was for %s", got.ID, id)
	}

	if _, err := s.Authenticate(ctx, "ada@acme.example", "the-new-password-ok"); err != nil {
		t.Fatalf("the new password does not work: %v", err)
	}
	if _, err := s.Authenticate(ctx, "ada@acme.example", "the-old-password-ok"); err == nil {
		t.Fatal("the old password still works — the reset changed nothing")
	}

	// The whole point of a link that arrives by email: it is worth one use. A
	// forwarded mail, a browser prefetch, or a back button are all a second
	// press of the same button.
	if _, err := s.CompleteReset(ctx, secret, "a-third-password-ok"); !errors.Is(err, identity.ErrReset) {
		t.Fatalf("the link worked twice (%v)", err)
	}
	if _, err := s.Authenticate(ctx, "ada@acme.example", "a-third-password-ok"); err == nil {
		t.Fatal("the second use changed the password anyway")
	}
}

/*
Every other outstanding link for the account goes with it.

Asking twice and clicking the first is ordinary — the second email is still in
the mailbox and would otherwise still work. It is also the shape of an attack
that has read one email: ask again, wait for the owner to reset, then use the
older link, which by then opens an account whose password the owner believes
only they know.
*/
func TestUsingOneResetSpendsTheOthers(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id := account(t, s, "ada@acme.example", "the-old-password-ok")

	first := asked(t, s, id, "ada@acme.example", time.Hour)
	second := asked(t, s, id, "ada@acme.example", time.Hour)

	if _, err := s.CompleteReset(ctx, second, "the-new-password-ok"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := s.CompleteReset(ctx, first, "somebody-elses-choice"); !errors.Is(err, identity.ErrReset) {
		t.Fatalf("an older link still worked after a newer one was used (%v)", err)
	}
}

func TestAnExpiredResetIsRefused(t *testing.T) {
	s := open(t)
	id := account(t, s, "ada@acme.example", "the-old-password-ok")

	// Written already expired, because the store's clock is fixed at StoreNow.
	secret := asked(t, s, id, "ada@acme.example", -time.Minute)

	if _, err := s.CompleteReset(context.Background(), secret, "the-new-password-ok"); !errors.Is(err, identity.ErrReset) {
		t.Fatalf("an expired link worked (%v)", err)
	}
}

/*
Disabled between asking and clicking.

Somebody walked out of a building at ten past four had a valid link in their
inbox at four. Checking only when the link is issued would let them back in
with it, and the account they come back into is one an administrator believes
is closed.
*/
func TestAResetForADisabledAccountIsRefused(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id := account(t, s, "ada@acme.example", "the-old-password-ok")
	secret := asked(t, s, id, "ada@acme.example", time.Hour)

	admin := principal.Principal{Subject: "boss", OrgID: "acme", ProjectID: "finance",
		ProjectRole: principal.ProjectAdmin}
	if err := s.SetDisabled(ctx, admin, id, true); err != nil {
		t.Fatal(err)
	}

	if _, err := s.CompleteReset(ctx, secret, "the-new-password-ok"); !errors.Is(err, identity.ErrReset) {
		t.Fatalf("a disabled account was reset back into (%v)", err)
	}
	// And the same answer as an expired one, so a held string never reveals
	// whose account it was for.
	if _, err := s.CompleteReset(ctx, "not-a-real-secret-at-all", "the-new-password-ok"); !errors.Is(err, identity.ErrReset) {
		t.Fatal("a secret nobody issued answered differently")
	}
}

/*
The sessions end with the password.

"I cannot get in" and "somebody else is in" are the same sentence from outside.
A token carries its own claims for eight hours and there is no list of sessions
to walk, so a line in time is the whole mechanism — and a reset that does not
draw one has recovered nothing from the person it was recovering from.
*/
func TestAResetEndsEverySessionTheAccountHad(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id := account(t, s, "ada@acme.example", "the-old-password-ok")

	if _, _, since := s.Active(ctx, id); !since.IsZero() {
		t.Fatalf("a fresh account already has a cut at %s", since)
	}

	secret := asked(t, s, id, "ada@acme.example", time.Hour)
	if _, err := s.CompleteReset(ctx, secret, "the-new-password-ok"); err != nil {
		t.Fatal(err)
	}

	known, active, since := s.Active(ctx, id)
	if !known || !active {
		t.Fatal("the account is no longer usable at all")
	}
	if since.IsZero() {
		t.Fatal("no line was drawn — every token minted before the reset still works")
	}
	if since.Before(StoreNow) {
		t.Fatalf("the line is at %s, before the reset at %s", since, StoreNow)
	}
}

// Pruning, because the row holds a hash of a credential tied to an address and
// there is no reason for it to outlive the hour the link was good for.
func TestSpentAndExpiredResetsArePruned(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id := account(t, s, "ada@acme.example", "the-old-password-ok")

	spent := asked(t, s, id, "ada@acme.example", time.Hour)
	if _, err := s.CompleteReset(ctx, spent, "the-new-password-ok"); err != nil {
		t.Fatal(err)
	}
	live := asked(t, s, id, "ada@acme.example", time.Hour)

	// Nothing yet: the grace is an hour and the clock has not moved, so a row
	// is never deleted out from under a request still holding it.
	if gone, err := s.PruneResets(ctx, time.Hour); err != nil || gone != 0 {
		t.Fatalf("pruned %d rows that were still within the grace (%v)", gone, err)
	}

	if gone, err := s.PruneResets(ctx, 0); err != nil {
		t.Fatal(err)
	} else if gone == 0 {
		t.Fatal("the spent link was never pruned")
	}

	// And the live one is still usable, which is what makes the prune safe.
	if _, err := s.CompleteReset(ctx, live, "a-later-password-ok"); err != nil {
		t.Fatalf("pruning took a link that was still good: %v", err)
	}
}
