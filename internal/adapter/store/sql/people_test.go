package sql_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
The store could create somebody and check their password, and nothing else.

That is enough to start a deployment and not enough to run one: when a person
leaves, the only way to take their access away was a SQL statement against
production, and the column that would have done it was in the schema from the
first migration and never written by anything.
*/

func TestDisablingRefusesTheNextSignIn(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	person := identity.User{
		ID: "usr_1", Email: "dewi@acme.example", Name: "Dewi",
		Org: "acme", Project: "finance", Role: "editor",
	}
	if err := s.CreateUser(ctx, person, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, person.Email, "correct horse battery staple"); err != nil {
		t.Fatalf("they could not sign in to begin with: %v", err)
	}

	if err := s.SetDisabled(ctx, acme, person.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, person.Email, "correct horse battery staple"); err == nil {
		t.Fatal("a disabled account signed in with the right password")
	}
	// And the running check that a live session is tested against.
	if known, active := s.Active(ctx, person.ID); !known || active {
		t.Fatalf("a disabled account: known=%v active=%v", known, active)
	}

	/* A subject that is not an account here is a machine credential — a
	   pipeline's token, or one baked into a portal build. Collapsing that into
	   "not allowed" locked every one of them out. */
	if known, _ := s.Active(ctx, "dewi"); known {
		t.Fatal("a subject nobody has was reported as an account")
	}

	// Reversible, because "disabled by mistake on their last day" is a thing
	// that happens and re-creating the account would lose their history.
	if err := s.SetDisabled(ctx, acme, person.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, person.Email, "correct horse battery staple"); err != nil {
		t.Fatalf("re-enabling did not: %v", err)
	}
}

// The row is kept. Deleting somebody removes the answer to "who ran this
// report in March", which a departure does not stop anybody asking.
func TestADisabledPersonIsStillOnTheRoster(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	if err := s.CreateUser(ctx, identity.User{
		ID: "usr_1", Email: "gone@acme.example",
		Org: "acme", Project: "finance", Role: "viewer",
	}, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDisabled(ctx, acme, "usr_1", true); err != nil {
		t.Fatal(err)
	}

	people, err := s.People(ctx, acme)
	if err != nil {
		t.Fatal(err)
	}
	if len(people) != 1 || !people[0].Disabled {
		t.Fatalf("roster is %+v", people)
	}
}

/*
The one that makes this a tenancy feature rather than a user feature.

An administrator of one project reading or changing another's people is the
failure the whole model exists to prevent, and it is prevented in the WHERE
clause rather than in a check somebody can forget.
*/
func TestOneProjectCannotSeeOrTouchAnothersPeople(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	if err := s.CreateUser(ctx, identity.User{
		ID: "usr_ours", Email: "ours@acme.example",
		Org: "acme", Project: "finance", Role: "admin",
	}, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}

	other := principal.Principal{OrgID: "globex", ProjectID: "ops", ProjectRole: principal.ProjectAdmin}

	people, err := s.People(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if len(people) != 0 {
		t.Fatalf("another project saw %d of our people", len(people))
	}
	if err := s.SetDisabled(ctx, other, "usr_ours", true); !errors.Is(err, identity.ErrNoUser) {
		t.Fatalf("another project disabled our person: %v", err)
	}
	if err := s.SetRole(ctx, other, "usr_ours", "viewer"); !errors.Is(err, identity.ErrNoUser) {
		t.Fatalf("another project changed our person's role: %v", err)
	}
	// And ours is untouched.
	if _, active := s.Active(ctx, "usr_ours"); !active {
		t.Fatal("our person was disabled by somebody else's administrator")
	}
}

/*
Changing a password takes the old one.

A session is eight hours long and lives in a browser. Without this, anybody who
borrowed one for a minute could lock the owner out of their own account
permanently — which is worse than the borrowed minute.
*/
func TestChangingAPasswordNeedsTheCurrentOne(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	if err := s.CreateUser(ctx, identity.User{
		ID: "usr_1", Email: "dewi@acme.example",
		Org: "acme", Project: "finance", Role: "editor",
	}, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}

	if err := s.ChangePassword(ctx, "usr_1", "not the password", "a whole new passphrase"); err == nil {
		t.Fatal("a wrong current password changed it anyway")
	}
	if _, err := s.Authenticate(ctx, "dewi@acme.example", "correct horse battery staple"); err != nil {
		t.Fatal("the old password stopped working after a failed change")
	}

	if err := s.ChangePassword(ctx, "usr_1", "correct horse battery staple", "a whole new passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, "dewi@acme.example", "a whole new passphrase"); err != nil {
		t.Fatalf("the new password does not work: %v", err)
	}
	if _, err := s.Authenticate(ctx, "dewi@acme.example", "correct horse battery staple"); err == nil {
		t.Fatal("the old password still works")
	}
}

// A disabled account cannot change its own password back into use.
func TestADisabledPersonCannotChangeTheirPassword(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	if err := s.CreateUser(ctx, identity.User{
		ID: "usr_1", Email: "gone@acme.example",
		Org: "acme", Project: "finance", Role: "editor",
	}, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDisabled(ctx, acme, "usr_1", true); err != nil {
		t.Fatal(err)
	}

	if err := s.ChangePassword(ctx, "usr_1", "correct horse battery staple", "a whole new passphrase"); err == nil {
		t.Fatal("a disabled account changed its own password")
	}
}
