package boot

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	_ "modernc.org/sqlite"
)

/*
Readiness, and which direction a wrong schema version points.

Readiness is not a health question, it is a routing question: a load balancer
reads it to decide whether to send work here. So it has to answer for the
instance that is answering, and the two ways a schema can be at the wrong
version are not the same thing at all.

Behind — a table this build reads is not there. Nothing here can be served, and
saying so is the point of the check. It is what a restore of an older dump
underneath a running process leaves.

Ahead — a newer cronos has migrated, which is every rolling deploy for the
length of the rollout. This reported down for that too, and the cost was not
theoretical: readiness is what keeps a pod in the load balancer, so the first
new instance to migrate pulled every old instance out at once, and a
multi-replica deployment served all of its traffic from the single new pod for
the rest of the rollout. scripts/live-upgrade.sh keeps an old build serving
across a real migration and finds every route still answering — reads,
publishes and sign-ins — because migrations only ever add tables.

Found by running the two versions side by side. No test could have found it,
because no test ran two versions.
*/
func TestReadinessAndASchemaAtAnotherVersion(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		what  string
		at    int
		ready bool
	}{
		{"at the version this build wants", sqlstore.Wanted(), true},
		{"ahead, because a newer cronos migrated", sqlstore.Wanted() + 1, true},
		{"far ahead, several versions on", sqlstore.Wanted() + 4, true},
		{"behind, as an older dump restored underneath", sqlstore.Wanted() - 1, false},
	} {
		t.Run(c.what, func(t *testing.T) {
			t.Parallel()

			store, db := migratedStore(t)
			setSchemaVersion(t, db, c.at)

			err := storeCheck(store).Probe(context.Background())

			switch {
			case c.ready && err != nil:
				t.Fatalf("not ready at schema %d, and this build wants %d: %v",
					c.at, sqlstore.Wanted(), err)
			case !c.ready && err == nil:
				t.Fatalf("ready at schema %d, which is behind the %d this build reads",
					c.at, sqlstore.Wanted())
			}

			// A probe that fails has to say which way round it is, because the
			// two have opposite answers: behind is restore the right dump,
			// ahead is finish the deploy.
			if err != nil && !strings.Contains(err.Error(), "needs") {
				t.Fatalf("the reason does not say what is missing: %v", err)
			}
		})
	}
}

// A store at the version this build migrates to, and a handle on the table the
// version is read from.
func migratedStore(t *testing.T) (*sqlstore.Store, *sql.DB) {
	t.Helper()

	// Shared cache: database/sql pools connections, and each connection to a
	// plain in-memory SQLite would open its own empty database.
	db, err := sql.Open("sqlite",
		fmt.Sprintf("file:readiness-%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := sqlstore.New(db, sqlstore.Question)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store, db
}

// Puts the recorded schema at a version, the way another build would have.
func setSchemaVersion(t *testing.T, db *sql.DB, at int) {
	t.Helper()
	ctx := context.Background()

	if _, err := db.ExecContext(ctx,
		"DELETE FROM cronos_schema_migrations WHERE id > ?", at); err != nil {
		t.Fatal(err)
	}
	// Ahead means rows this build has no migration for — exactly what a newer
	// one leaves behind.
	for id := sqlstore.Wanted() + 1; id <= at; id++ {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO cronos_schema_migrations (id, name, applied_at) VALUES (?, ?, ?)",
			id, fmt.Sprintf("from-a-newer-build-%d", id), "2026-08-16T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}

	var got int
	if err := db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(id), 0) FROM cronos_schema_migrations").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != at {
		t.Fatalf("could not put the schema at %d — it is at %d", at, got)
	}
}
