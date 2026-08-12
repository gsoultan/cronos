package definition

import "time"

/*
Pool bounds the connections cronos holds open to one source.

Somebody else operates that database. A reporting tool that opens two hundred
connections during a burst is a reporting tool their DBA turns off — and until
these defaults existed that is exactly what it did: MaxOpen was only applied
when a definition set it, so a source that said nothing got database/sql's
default, which is unlimited.

Every field is optional and every default is a number a deployment can raise
when it owns the database it is reading.
*/
type Pool struct {
	// MaxOpen is the most connections open at once. Zero means DefaultMaxOpen,
	// not unlimited.
	MaxOpen int `json:"maxOpen,omitempty" yaml:"maxOpen,omitempty"`
	// MaxIdle is how many are kept when the work stops. Zero means as many as
	// MaxOpen.
	//
	// database/sql keeps two by default, which under load means a connection
	// is opened and closed for almost every query — a TCP handshake, a TLS
	// negotiation and an authentication round trip, per report block. Keeping
	// them matched to MaxOpen is what makes a pool a pool.
	MaxIdle int `json:"maxIdle,omitempty" yaml:"maxIdle,omitempty"`
	// MaxIdleTime releases a connection nobody has used. Zero means
	// DefaultMaxIdleTime, so a quiet deployment is not holding connections
	// open against somebody else's limit all night.
	MaxIdleTime Duration `json:"maxIdleTime,omitempty" yaml:"maxIdleTime,omitempty"`
	// MaxLifetime retires a connection regardless of use. Zero means
	// DefaultMaxLifetime.
	//
	// The one nobody thinks of until it happens: a load balancer, a failover
	// or a database restart kills connections without telling the client, and
	// a pool with no lifetime hands out a dead one and reports it as a query
	// error. A bounded lifetime is also how a rotated password takes effect
	// without a restart.
	MaxLifetime Duration `json:"maxLifetime,omitempty" yaml:"maxLifetime,omitempty"`
}

// The defaults, which are a judgement about somebody else's database.
const (
	// DefaultMaxOpen is enough to keep a reporting workload busy and small
	// enough that several cronos instances do not exhaust a Postgres default
	// of a hundred connections between them. Measured: throughput peaks around
	// sixteen concurrent renders on this shape of query and falls after.
	DefaultMaxOpen = 16
	// DefaultMaxIdleTime releases connections a quiet deployment is not using.
	DefaultMaxIdleTime = 5 * time.Minute
	// DefaultMaxLifetime is short enough that a failover or a rotated
	// credential is picked up without a restart, and long enough that
	// reconnecting is not part of the steady state.
	DefaultMaxLifetime = 30 * time.Minute
)

// Open is the most connections to hold at once.
func (p Pool) Open() int {
	if p.MaxOpen > 0 {
		return p.MaxOpen
	}
	return DefaultMaxOpen
}

// Idle is how many to keep when the work stops.
func (p Pool) Idle() int {
	if p.MaxIdle > 0 {
		return p.MaxIdle
	}
	// Matched to Open rather than to two: a pool that closes a connection the
	// moment a burst pauses reopens it a millisecond later.
	return p.Open()
}

// IdleFor is how long an unused connection is kept.
func (p Pool) IdleFor() time.Duration {
	if p.MaxIdleTime > 0 {
		return time.Duration(p.MaxIdleTime)
	}
	return DefaultMaxIdleTime
}

// LifetimeOf is how long any connection is kept.
func (p Pool) LifetimeOf() time.Duration {
	if p.MaxLifetime > 0 {
		return time.Duration(p.MaxLifetime)
	}
	return DefaultMaxLifetime
}
