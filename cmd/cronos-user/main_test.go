package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"

	_ "modernc.org/sqlite"
)

/*
TestPlatformGrantReachesTheAccountItJustCreated covers the fresh install, which
is one of the two cases this command exists for.

create mints the ID, and it used to mint it into its own copy of the user — so
`-platform` on an account that did not exist yet granted the empty string. The
account was created, the grant came back "identity: no such person", and the
command exited 1: a deployment with an administrator who cannot administer the
deployment, reported as a failure, after a partial success nobody could see.
*/
func TestPlatformGrantReachesTheAccountItJustCreated(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "cronos.db")
	onStdin(t, "a-development-passphrase")

	err := run("sqlite", dsn, identity.User{
		Email: "dev@cronos.local", Name: "Dev Admin",
		Org: "default", Project: "default", Role: "admin",
	}, true, false)
	if err != nil {
		t.Fatalf("creating a first administrator with -platform: %v", err)
	}

	ctx := context.Background()
	store := open2(t, dsn)
	user, err := store.ByEmail(ctx, "dev@cronos.local")
	if err != nil {
		t.Fatalf("reading back the account that was just created: %v", err)
	}
	if !store.IsPlatformAdmin(ctx, user.ID) {
		t.Errorf("%s was created but is not a deployment administrator", user.Email)
	}
}

// open2 opens the store the command wrote, to read back what it did.
func open2(t *testing.T, dsn string) *sqlstore.Store {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlstore.New(db, sqlstore.Question).ForDriver("sqlite")
}

// onStdin hands the command a password the way a pipe does. The terminal branch
// of readPassword probes file descriptor 0, which `go test` does not attach a
// terminal to, so this is the path taken here and in every script that seeds a
// development deployment.
func onStdin(t *testing.T, password string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(password + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	was := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = was; _ = r.Close() })
}

/*
TestIfEmptyLeavesADeploymentWithAccountsAlone is what makes the flag safe to run
on every start.

The provisioning case wants "make sure somebody can sign in", not "make another
administrator" — so a store that already has one is left as it is, and saying so
is not an error a start-up script has to special-case.
*/
func TestIfEmptyLeavesADeploymentWithAccountsAlone(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "cronos.db")
	ctx := context.Background()

	onStdin(t, "the-first-passphrase")
	someone := identity.User{
		Email: "ada@example.com", Name: "Ada",
		Org: "acme", Project: "finance", Role: "admin",
	}
	if err := run("sqlite", dsn, someone, false, false); err != nil {
		t.Fatalf("creating the account that is already there: %v", err)
	}

	onStdin(t, "a-development-passphrase")
	seeded := identity.User{
		Email: "dev@cronos.local", Name: "Dev",
		Org: "default", Project: "default", Role: "admin",
	}
	if err := run("sqlite", dsn, seeded, true, true); err != nil {
		t.Fatalf("-if-empty on a deployment that has an account: %v", err)
	}

	store := open2(t, dsn)
	if _, err := store.ByEmail(ctx, seeded.Email); err == nil {
		t.Errorf("-if-empty created %s in a deployment that already had %s",
			seeded.Email, someone.Email)
	}
	if _, err := store.ByEmail(ctx, someone.Email); err != nil {
		t.Errorf("%s went missing: %v", someone.Email, err)
	}
}
