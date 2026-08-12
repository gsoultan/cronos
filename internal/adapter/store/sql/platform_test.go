package sql_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
The first run, decided by the database.

The handler's guarantee used to be a mutex, which is enough for a double-clicked
button and not enough for two cronos processes brought up against one empty
database before anybody had the address. Both would find it empty and both would
create a deployment administrator — and the second one is a credential nobody
issued, on a system nobody is watching yet.

These tests use one *Store, which is the same database from the transaction's
point of view. What they prove is that the marker row decides, not the caller.
*/

func first(id, email string) identity.User {
	return identity.User{
		ID: id, Email: email, Name: "Ada",
		Org: "acme", Project: "finance", Role: "admin",
	}
}

func TestAFirstRunMakesAnAdministratorAndSaysItHappened(t *testing.T) {
	both(t, func(t *testing.T, s *store.Store) {

		if done, err := s.SetUp(context.Background()); err != nil || done {
			t.Fatalf("a fresh deployment says it is set up: %v %v", done, err)
		}

		if err := s.FirstRun(context.Background(),
			first("usr_ada", "ada@acme.example"), "a-password-they-chose"); err != nil {
			t.Fatal(err)
		}

		if done, err := s.SetUp(context.Background()); err != nil || !done {
			t.Fatalf("after a first run it says %v (%v)", done, err)
		}
		if !s.IsPlatformAdmin(context.Background(), "usr_ada") {
			t.Fatal("the first account does not administer the deployment")
		}
		// And it can sign in, which is the half a grant without an account loses.
		if _, err := s.Authenticate(context.Background(),
			"ada@acme.example", "a-password-they-chose"); err != nil {
			t.Fatalf("the first account cannot sign in: %v", err)
		}
	})
}

/*
Two at once produce one administrator, whatever the addresses.

Different emails on purpose: the unique index on email would catch identical
ones, and then this would be testing that index rather than the marker row.
*/
func TestTwoFirstRunsAgainstOneDatabaseProduceOne(t *testing.T) {
	both(t, func(t *testing.T, s *store.Store) {

		const tries = 8
		var (
			wg    sync.WaitGroup
			mu    sync.Mutex
			won   int
			other int
		)
		start := make(chan struct{})
		for i := range tries {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				id := "usr_" + string(rune('a'+i))
				err := s.FirstRun(context.Background(),
					first(id, id+"@acme.example"), "a-password-they-chose")

				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					won++
				case errors.Is(err, store.ErrAlreadySetUp):
				default:
					other++
				}
			}()
		}
		close(start)
		wg.Wait()

		if won != 1 {
			t.Fatalf("%d of %d concurrent first runs succeeded", won, tries)
		}
		if other != 0 {
			t.Fatalf("%d losers failed for a reason other than being second", other)
		}

		admins, err := s.PlatformAdmins(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(admins) != 1 {
			t.Fatalf("%d platform administrators exist", len(admins))
		}
		people, err := s.EveryPerson(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(people) != 1 {
			t.Fatalf("%d accounts were created", len(people))
		}
	})
}

/*
A first run that loses writes nothing at all.

Not merely "no administrator" — no account either. An address consumed by a
losing attempt is an address its owner cannot then be invited under, and the
person who lost the race has no way to know why.
*/
func TestALosingFirstRunLeavesNoTrace(t *testing.T) {
	both(t, func(t *testing.T, s *store.Store) {

		if err := s.FirstRun(context.Background(),
			first("usr_ada", "ada@acme.example"), "a-password-they-chose"); err != nil {
			t.Fatal(err)
		}
		err := s.FirstRun(context.Background(),
			first("usr_grace", "grace@acme.example"), "another-password-here")
		if !errors.Is(err, store.ErrAlreadySetUp) {
			t.Fatalf("the second run answered %v", err)
		}

		people, err := s.EveryPerson(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(people) != 1 {
			t.Fatalf("the losing run left %d accounts", len(people))
		}
		if people[0].Email != "ada@acme.example" {
			t.Fatalf("the account is %s", people[0].Email)
		}
	})
}

/*
A deployment that predates the marker is not offered a first run.

Migration 7 shipped after people were already running cronos, so an existing
deployment has accounts and no marker row. Without counting them, every upgrade
would offer its next visitor a deployment administrator.
*/
func TestAnUpgradedDeploymentIsNotOfferedAFirstRun(t *testing.T) {
	s := open(t)

	// An account made the way every version before this made them.
	if err := s.CreateUser(context.Background(), identity.User{
		ID: "usr_ada", Email: "ada@acme.example",
		Org: "acme", Project: "finance", Role: "admin",
	}, "a-password-they-chose"); err != nil {
		t.Fatal(err)
	}

	done, err := s.SetUp(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("a deployment with accounts and no marker offers to be set up")
	}
}

/*
A first run that cannot reach the database is not a first run that lost.

The marker insert used to map every error to "this deployment has already been
set up", which is the one answer that makes somebody stop trying — they would
read it on a brand-new deployment whose database was briefly unreachable and go
looking for an account that does not exist.

Provoked by taking the marker table away, so the insert itself fails for a
reason that is plainly not a collision. Closing the whole database would be
easier and would prove nothing: it fails at BeginTx, several lines before the
branch under test — which is what the first version of this test did, and a
mutation that broke the branch left it passing.
*/
func TestAFailedFirstRunIsNotReportedAsASecondOne(t *testing.T) {
	s, db := fresh(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE cronos_setup`); err != nil {
		t.Fatal(err)
	}

	err := s.FirstRun(context.Background(),
		first("usr_ada", "ada@acme.example"), "a-password-they-chose")

	if err == nil {
		t.Fatal("a first run with no marker table succeeded")
	}
	if errors.Is(err, store.ErrAlreadySetUp) {
		t.Fatalf("a broken database was reported as a deployment already set up: %v", err)
	}
	// And nothing was written, because the transaction rolled back.
	if n, _ := s.CountAccounts(context.Background()); n != 0 {
		t.Fatalf("a failed first run left %d accounts", n)
	}
}
