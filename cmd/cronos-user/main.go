// Command cronos-user creates the first person who can sign in.
//
// A command rather than a first-run web form. A deployment that shows a
// "create the first admin" page to whoever reaches it first is a deployment
// where the first visitor is whoever found the port, and the window between
// starting the server and remembering to visit it is a real one.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
	"golang.org/x/term"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func main() {
	var (
		email   = flag.String("email", "", "who they sign in as")
		name    = flag.String("name", "", "what to call them")
		org     = flag.String("org", "", "organization")
		project = flag.String("project", "", "project")
		role    = flag.String("role", "editor", "admin, editor or viewer")
		driver  = flag.String("driver", envOr("CRONOS_STORE_DRIVER", "postgres"), "store driver")
		dsn     = flag.String("dsn", os.Getenv("CRONOS_STORE_DSN"), "store dsn")

		/*
		   platform makes this account a deployment administrator.

		   The way back from having none. The endpoints that grant it require
		   the permission being granted, so a deployment whose last one was
		   revoked, disabled or lost cannot make another over HTTP — and the
		   documentation said the remedy was this command before this command
		   could do it, which is a recovery path that existed only in prose.

		   Deliberately grant-only. Revoking is the API's job, where the check
		   that stops the last one going lives; a CLI that could remove it would
		   be a way to create the state this flag exists to escape.
		*/
		platform = flag.Bool("platform", false,
			"also make them a deployment administrator, or grant it to an existing account")
	)
	flag.Parse()

	if err := run(*driver, *dsn, identity.User{
		Email: *email, Name: *name, Org: *org, Project: *project, Role: *role,
	}, *platform); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

/*
run creates an account, grants deployment administration, or both.

Which of the three depends on what is asked for and what is already there. The
common cases are a first account on a fresh install, and `-email … -platform` on
a deployment that has locked itself out — and the second must work without
asking for a password the person may not know, because the account it is
rescuing is somebody else's.
*/
func run(driver, dsn string, u identity.User, platform bool) error {
	store, err := open(driver, dsn)
	if err != nil {
		return err
	}
	defer store.close()

	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		return err
	}

	if u.Email == "" {
		return fmt.Errorf("-email is required")
	}

	existing, err := store.ByEmail(ctx, u.Email)
	switch {
	case err == nil && !platform:
		return fmt.Errorf("%s already has an account — use -platform to make them "+
			"a deployment administrator, or the portal to change their role", u.Email)

	case err == nil:
		// Known, and being granted. No password is asked for: this is somebody
		// else's account being rescued, and a command that reset the password
		// to grant a permission would be a command that locks its owner out to
		// let them back in.
		if err := store.GrantPlatform(ctx, existing.ID, "cronos-user"); err != nil {
			return err
		}
		fmt.Printf("%s is now a deployment administrator\n", existing.Email)
		return nil
	}

	if err := create(ctx, store, u); err != nil {
		return err
	}
	if platform {
		if err := store.GrantPlatform(ctx, u.ID, "cronos-user"); err != nil {
			return err
		}
		fmt.Printf("and a deployment administrator\n")
	}
	return nil
}

// create makes a new account, asking for a password at the terminal.
func create(ctx context.Context, store *opened, u identity.User) error {
	if u.Org == "" || u.Project == "" {
		return fmt.Errorf("-org and -project are required: a user acts somewhere")
	}

	password, err := readPassword()
	if err != nil {
		return err
	}

	u.ID = identity.NewID()
	if err := store.CreateUser(ctx, u, password); err != nil {
		return err
	}
	fmt.Printf("created %s as %s in %s/%s\n", u.Email, u.Role, u.Org, u.Project)
	return nil
}

// opened is the store and the handle to close.
type opened struct {
	*sqlstore.Store
	db *sql.DB
}

func (o *opened) close() { _ = o.db.Close() }

func open(driver, dsn string) (*opened, error) {
	if dsn == "" {
		return nil, fmt.Errorf("set CRONOS_STORE_DSN — users live in the definition store")
	}

	name := driver
	if name == "postgres" {
		name = "pgx"
	}
	db, err := sql.Open(name, dsn)
	if err != nil {
		return nil, err
	}

	mark := sqlstore.Question
	if driver == "postgres" || driver == "pgx" {
		mark = sqlstore.Dollar
	}
	return &opened{Store: sqlstore.New(db, mark).ForDriver(driver), db: db}, nil
}

// readPassword takes it from the terminal without echoing, or from stdin when
// there is no terminal.
//
// Never from a flag. A password on a command line is in the shell history, in
// the process list, and in whatever shipped that shell history somewhere.
func readPassword() (string, error) {
	if term.IsTerminal(int(syscall.Stdin)) {
		fmt.Fprint(os.Stderr, "password: ")
		typed, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		return string(typed), err
	}

	// A whole line, not a whitespace-delimited token. The minimum length
	// encourages a passphrase, and "correct horse battery staple" read with
	// Fscanln is the word "correct" and an error.
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("no password on stdin")
	}
	return strings.TrimRight(scanner.Text(), "\r\n"), scanner.Err()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
