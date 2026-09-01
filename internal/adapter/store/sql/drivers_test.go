package sql_test

import (
	"os"
	"testing"
	"time"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
)

/*
Tests whose subject is what the database does, run on every database we support.

Most of this package's tests are about logic that happens to be written in SQL —
that every read carries the tenant, that a version is content-addressed, that
history outlives a delete. Those are the same on either driver and run on SQLite,
which needs no container and starts in a millisecond.

A handful are not. When a test's subject is "two of these at once produce one of
those", SQLite is the wrong place to ask: it serialises writes, so a conditional
UPDATE, an ON CONFLICT and a plain read-then-write all look identical. Every
concurrency guarantee in this store was written against it and passed, which
proves those guarantees hold on a database nobody deploys.

Postgres is where they mean something: real concurrent writers, read-committed
isolation, and its own rules about what RowsAffected counts after an ON CONFLICT.
So the tests below run on both, and the Postgres half is skipped loudly rather
than silently when there is no server — a skip that reads like a pass is how a
gap like this survives.

	both(t, func(t *testing.T, s *store.Store) {
		// ... the test, against whichever driver this subtest is
	})
*/
func both(t *testing.T, run func(t *testing.T, s *store.Store)) {
	t.Helper()

	t.Run("sqlite", func(t *testing.T) {
		run(t, at(open(t)))
	})

	t.Run("postgres", func(t *testing.T) {
		if os.Getenv("CRONOS_POSTGRES_DSN") == "" {
			// Loud, and naming what is not being checked rather than the
			// variable that would check it. "postgres skipped" in a log is
			// something people learn to read past.
			t.Skip("set CRONOS_POSTGRES_DSN — without it this guarantee is only " +
				"proved on SQLite, which serialises writes and cannot show a race")
		}
		// Its own schema per subtest, so the two halves cannot see each other's
		// rows and a failure names the driver it happened on.
		run(t, at(postgres(t)))
	})
}

/*
StoreNow is the moment both halves run at.

They have to agree, and they did not: open() pinned the clock and postgres() read
the real one, so a test computing a TOTP code at this instant passed on SQLite
and failed on Postgres with "that code is not right" — a difference in the
harness reported as a difference in the store. Found the first time the two ran
side by side, which is the argument for running them side by side.
*/
var StoreNow = time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)

func at(s *store.Store) *store.Store {
	return s.WithClock(func() time.Time { return StoreNow })
}
