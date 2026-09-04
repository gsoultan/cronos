package sql_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"

	_ "modernc.org/sqlite"
)

/*
The executor every query in the product passes through.

It had no test file. What it does is small and each piece of it is the last
thing standing between a report and the database somebody else operates: the
row cap that decides whether a query over a ten-million-row table takes the
server down, the statement timeout that decides whether a runaway query costs a
request or a shift, and the refusal to run a plan that was never compiled —
which is what stops a zero Plan reaching a driver as an empty statement.

Exercised through a real database rather than a fake. A row cap asserted
against a stub proves the arithmetic and not the contract: the wrapper has to
report the cap through Err(), because a caller reading a result set correctly
checks Err() and nothing else, and a truncation that returns nil there is a
wrong answer presented as a right one.
*/

func openDB(t *testing.T, rows int) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// One connection, so :memory: is one database rather than one per
	// pooled connection with the table on whichever got the CREATE.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE invoices (id TEXT, total REAL)`); err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if _, err := db.Exec(`INSERT INTO invoices VALUES (?, ?)`, string(rune('a'+i)), i); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// plan compiles sql through the builder, because Plan's statement is
// unexported and only the builder sets it — that is the guarantee the executor
// relies on, so a test may not fabricate one either.
func plan(t *testing.T, statement string) query.Plan {
	t.Helper()

	ds := definition.Dataset{
		Name:    "invoices",
		Sources: []definition.SourceRef{{Ref: "warehouse"}},
		Query:   statement,
		Fields: []definition.Field{
			{Name: "id", Type: "string", Role: definition.Dimension},
		},
	}
	p, err := query.NewBuilder(query.SQLite{}).Build(ds, nil, admin())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return p
}

func admin() principal.Principal {
	return principal.Principal{
		Subject: "u1", OrgID: "o1", ProjectID: "p1",
		ProjectRole: principal.ProjectAdmin,
	}
}

// drain reads a result set the way a caller should: to exhaustion, then Err.
func drain(t *testing.T, rows interface {
	Next() bool
	Err() error
	Close() error
}) (int, error) {
	t.Helper()
	defer rows.Close()

	n := 0
	for rows.Next() {
		n++
	}
	return n, rows.Err()
}

func TestTheRowCapRefusesRatherThanTruncating(t *testing.T) {
	e := sqldriver.NewExecutor(openDB(t, 10)).
		WithLimits(definition.Limits{MaxRows: 4})

	rows, err := e.Execute(context.Background(), plan(t, `SELECT id FROM invoices`))
	if err != nil {
		t.Fatal(err)
	}

	read, err := drain(t, rows)
	if !errors.Is(err, sqldriver.ErrTooManyRows) {
		t.Fatalf("a result set over the cap returned %v, want ErrTooManyRows — "+
			"a caller checking Err() would have seen a clean finish", err)
	}
	if read != 4 {
		t.Errorf("read %d rows, want the cap of 4", read)
	}
	if !strings.Contains(err.Error(), "4") {
		t.Errorf("the message should name the cap: %v", err)
	}
}

// The boundary. A cap of exactly the number of rows is not an overflow, and
// getting this off by one turns the limit into a limit minus one.
func TestAResultSetExactlyAtTheCapIsFine(t *testing.T) {
	e := sqldriver.NewExecutor(openDB(t, 4)).
		WithLimits(definition.Limits{MaxRows: 4})

	rows, err := e.Execute(context.Background(), plan(t, `SELECT id FROM invoices`))
	if err != nil {
		t.Fatal(err)
	}

	read, err := drain(t, rows)
	if err != nil {
		t.Fatalf("exactly the cap was refused: %v", err)
	}
	if read != 4 {
		t.Errorf("read %d rows, want 4", read)
	}
}

// A datasource that says nothing still gets a cap.
func TestASourceWithNoLimitsStillHasARowCap(t *testing.T) {
	e := sqldriver.NewExecutor(openDB(t, 3))

	if e.MaxRows != 0 {
		t.Fatalf("the fixture should carry no explicit cap, got %d", e.MaxRows)
	}
	rows, err := e.Execute(context.Background(), plan(t, `SELECT id FROM invoices`))
	if err != nil {
		t.Fatal(err)
	}
	if read, err := drain(t, rows); err != nil || read != 3 {
		t.Fatalf("read %d rows, err %v — want 3 and nil under the default cap", read, err)
	}
}

func TestWithLimitsTakesTheDatasourcesOwnBounds(t *testing.T) {
	e := sqldriver.NewExecutor(openDB(t, 1)).WithLimits(definition.Limits{
		MaxRows: 17, StatementTimeout: definition.Duration(5 * time.Second),
	})

	if e.MaxRows != 17 {
		t.Errorf("MaxRows is %d, want 17", e.MaxRows)
	}
	if e.Timeout != 5*time.Second {
		t.Errorf("Timeout is %s, want 5s", e.Timeout)
	}
}

/*
The zero Plan is refused rather than sent.

A Plan whose statement is empty never came from the builder, which is the only
thing that applies row scope. Sending it and reporting whatever the driver says
about an empty statement would turn a missing compile step into a syntax error
from Postgres.
*/
func TestAnUncompiledPlanIsNeverExecuted(t *testing.T) {
	e := sqldriver.NewExecutor(openDB(t, 1))

	if _, err := e.Execute(context.Background(), query.Plan{}); err == nil {
		t.Fatal("the zero Plan was executed")
	}
	if err := e.Verify(context.Background(), query.Plan{}); err == nil {
		t.Fatal("the zero Plan was verified")
	}
}

// The statement timeout is applied, not merely stored. An already-expired
// deadline is used rather than a slow query, so the assertion is about the
// wiring and not about how fast this machine happens to be.
func TestTheStatementTimeoutBoundsTheQuery(t *testing.T) {
	e := sqldriver.NewExecutor(openDB(t, 1)).
		WithLimits(definition.Limits{StatementTimeout: definition.Duration(1)})

	_, err := e.Execute(context.Background(), plan(t, `SELECT id FROM invoices`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want the statement's own deadline to have bounded it", err)
	}
}

/*
Verify resolves names without reading rows.

This is the check that catches a column the dataset declares and the warehouse
does not have — the class of failure compiling a block cannot see, and the one
that otherwise surfaces at 06:00 in a burst with the driver's own words for it.
*/
func TestVerifyRefusesAStatementTheDatabaseCannotResolve(t *testing.T) {
	e := sqldriver.NewExecutor(openDB(t, 1))

	err := e.Verify(context.Background(), plan(t, `SELECT nonexistent FROM invoices`))
	if err == nil {
		t.Fatal("a statement naming a column the database has not got was verified")
	}
}

func TestVerifyAcceptsAStatementTheDatabaseWillTake(t *testing.T) {
	e := sqldriver.NewExecutor(openDB(t, 1))

	if err := e.Verify(context.Background(), plan(t, `SELECT id FROM invoices`)); err != nil {
		t.Fatalf("a statement the database accepts was refused: %v", err)
	}
}

// And it touches no rows. Verifying a statement that would delete everything
// leaves everything, because a prepare is not an execution — this is what
// makes it cheap enough to do on every publish.
func TestVerifyRunsNothing(t *testing.T) {
	db := openDB(t, 3)
	e := sqldriver.NewExecutor(db)

	if err := e.Verify(context.Background(), plan(t, `DELETE FROM invoices`)); err != nil {
		t.Fatalf("verify: %v", err)
	}

	var left int
	if err := db.QueryRow(`SELECT count(*) FROM invoices`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 3 {
		t.Fatalf("%d rows left, want 3 — Verify executed the statement", left)
	}
}

// Closing a result set releases the statement's context. Without it every
// query holds a timer and a context until the timeout fires, which on a busy
// instance is memory growth with no owner.
func TestClosingRowsIsCleanAndRepeatable(t *testing.T) {
	e := sqldriver.NewExecutor(openDB(t, 2))

	rows, err := e.Execute(context.Background(), plan(t, `SELECT id FROM invoices`))
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// A caller that closes and then defers a close must not panic on the
	// second cancel, which is the ordinary shape of this code.
	if err := rows.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
