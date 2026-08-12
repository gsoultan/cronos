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
	)
	flag.Parse()

	if err := create(*driver, *dsn, identity.User{
		Email: *email, Name: *name, Org: *org, Project: *project, Role: *role,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func create(driver, dsn string, u identity.User) error {
	switch {
	case dsn == "":
		return fmt.Errorf("set CRONOS_STORE_DSN — users live in the definition store")
	case u.Email == "":
		return fmt.Errorf("-email is required")
	case u.Org == "" || u.Project == "":
		return fmt.Errorf("-org and -project are required: a user acts somewhere")
	}

	password, err := readPassword()
	if err != nil {
		return err
	}

	open := driver
	if open == "postgres" {
		open = "pgx"
	}
	db, err := sql.Open(open, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	mark := sqlstore.Question
	if driver == "postgres" || driver == "pgx" {
		mark = sqlstore.Dollar
	}
	store := sqlstore.New(db, mark).ForDriver(driver)

	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	u.ID = identity.NewID()
	if err := store.CreateUser(ctx, u, password); err != nil {
		return err
	}

	fmt.Printf("created %s as %s in %s/%s\n", u.Email, u.Role, u.Org, u.Project)
	return nil
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
