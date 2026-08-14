package sql

import (
	"context"
	"database/sql"
	"hash/fnv"
	"sync"
)

/*
Leadership, so several replicas can all be armed and one of them schedules.

CRONOS_SCHEDULER had to be on for exactly one instance, which is a rule a
deployment holds in its head. Set it on two and every customer gets two copies
of their statement; forget it entirely and nobody gets one. Both are quiet:
double delivery is noticed by the recipient, and no delivery is noticed by the
recipient a month later.

A Postgres advisory lock is the whole mechanism, and its liveness is the part
worth understanding: the lock belongs to a session, so it is released when that
session ends — including when the process holding it is killed, because the
operating system closes the socket and Postgres notices. There is no lease to
expire and no clock to agree on, which is why this is preferred to a table with
a timestamp in it. Two instances cannot both hold it; that is the database's
guarantee rather than ours.

What it does not give is instant failover in every case. A host that freezes
without closing its sockets holds the lock until Postgres' keepalives give up,
which is minutes. That is the safe direction — nobody schedules for a while,
rather than two instances scheduling at once — and the gauges added alongside
this make the gap visible while it lasts.
*/

// Lease is a leadership claim on one name, held on a connection of its own.
type Lease struct {
	db   *sql.DB
	name string
	key  int64

	mu   sync.Mutex
	conn *sql.Conn
}

/*
Lease returns a claim on name, or nil where the store cannot arbitrate one.

Nil for SQLite, which has no advisory locks and needs none: a SQLite deployment
is one process by construction, so it leads unconditionally. A caller treats nil
as "you are the only one here", which is true.
*/
func (s *Store) Lease(name string) *Lease {
	if s.driver != "postgres" && s.driver != "pgx" {
		return nil
	}
	return &Lease{db: s.db, name: name, key: leaseKey(name)}
}

/*
Leading reports whether this process holds the claim, taking it if it can.

Cheap when already held — one round trip to confirm the session is alive — and
one attempt when not. Called on the scheduler's own tick, so the cost is a
statement a minute rather than anything a warehouse would notice.

The ping matters. A connection can die without this process noticing: the
database restarts, a proxy drops an idle session, a network blips. The lock went
with it, so continuing to believe otherwise is exactly the split brain this
exists to prevent — another replica will have taken it by then.
*/
func (l *Lease) Leading(ctx context.Context) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn != nil {
		if err := l.conn.PingContext(ctx); err == nil {
			return true
		}
		// The session is gone and so is the lock. Drop it and try again below
		// rather than assuming either way.
		_ = l.conn.Close()
		l.conn = nil
	}

	conn, err := l.db.Conn(ctx)
	if err != nil {
		return false
	}
	var held bool
	// try_ rather than the blocking form: a follower asks and moves on. Waiting
	// would mean a replica stuck in a query for as long as the leader lives.
	if err := conn.QueryRowContext(ctx,
		"SELECT pg_try_advisory_lock($1)", l.key).Scan(&held); err != nil || !held {
		_ = conn.Close()
		return false
	}
	l.conn = conn
	return true
}

// Release gives the claim up, so a replacement can take it without waiting for
// the database to notice a closed socket. Called on the way out of a graceful
// shutdown; a process that dies without it releases the lock anyway.
func (l *Lease) Release() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil {
		return
	}
	// Best effort. Closing the connection releases the lock regardless, which
	// is why the error is not worth reporting to anybody.
	_, _ = l.conn.ExecContext(context.Background(),
		"SELECT pg_advisory_unlock($1)", l.key)
	_ = l.conn.Close()
	l.conn = nil
}

/*
leaseKey turns a name into the number Postgres locks on.

A 64-bit FNV-1a, so two tenants in one deployment colliding is not a thing that
happens — the 32-bit two-key form would collide at a few tens of thousands of
tenants, and a collision means one project silently never schedules, which is
the failure mode least likely to be noticed.

The prefix keeps these out of the way of any other advisory lock a deployment
uses, including this store's own migration lock.
*/
func leaseKey(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("cronos:lease:" + name))
	return int64(h.Sum64()) //nolint:gosec // a lock key is a number, not a size
}
