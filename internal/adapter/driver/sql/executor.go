package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

// Executor runs plans against one database.
type Executor struct {
	db *sql.DB
	// MaxRows bounds what a single query may return. Zero means
	// definition.DefaultMaxRows.
	//
	// The datasource's own limit, not a global: somebody else operates that
	// database and decided what a reasonable answer from it looks like.
	MaxRows int
	// Timeout bounds a single statement. Zero means DefaultTimeout.
	//
	// Never unbounded: a report is a query someone wrote against a database
	// someone else operates, and the failure mode of an unbounded one is a
	// connection pool held by a statement nobody is waiting for any more.
	Timeout time.Duration
}

// DefaultTimeout is long enough for a real analytical query and short enough
// that a runaway one fails a request rather than a shift.
const DefaultTimeout = 30 * time.Second

// NewExecutor returns an Executor over db.
func NewExecutor(db *sql.DB) *Executor { return &Executor{db: db} }

// WithLimits applies a datasource's own bounds.
func (e *Executor) WithLimits(l definition.Limits) *Executor {
	return &Executor{db: e.db, MaxRows: l.Rows(), Timeout: l.Timeout()}
}

// Execute runs p.
//
// It takes a query.Plan, which is the guarantee: the SQL inside one is
// unexported and only the builder sets it, so nothing can reach this method
// with a statement that skipped row scope.
func (e *Executor) Execute(ctx context.Context, p query.Plan) (run.Rows, error) {
	if p.Empty() {
		// The zero Plan. Refusing beats sending an empty statement and
		// reporting whatever the driver says about it.
		return nil, fmt.Errorf("sql: refusing to execute an uncompiled plan")
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)

	rows, err := e.db.QueryContext(ctx, p.SQL(), p.Args()...)
	if err != nil {
		cancel()
		return nil, err
	}
	limit := e.MaxRows
	if limit <= 0 {
		limit = definition.DefaultMaxRows
	}
	// The context outlives this call because the caller reads from rows, so
	// cancellation is tied to closing them rather than to returning.
	return &capped{Rows: &cancelling{Rows: rows, cancel: cancel}, limit: limit}, nil
}

// cancelling releases the statement's context when the caller is finished.
//
// Without it the timeout keeps a timer and a context alive per query until it
// fires, which on a busy instance is a slow leak that looks like memory growth
// with no owner.
type cancelling struct {
	*sql.Rows
	cancel context.CancelFunc
}

func (c *cancelling) Close() error {
	err := c.Rows.Close()
	c.cancel()
	return err
}
