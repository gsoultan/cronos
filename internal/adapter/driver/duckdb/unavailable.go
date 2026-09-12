//go:build !duckdb

package duckdb

import (
	"context"

	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/core/definition"
)

// Federation is the shape the tagged build provides.
type Federation struct{}

// Open always fails in an untagged build.
func Open(context.Context, map[string]definition.DataSource) (*Federation, error) {
	return nil, ErrNotBuilt
}

// Close is a no-op.
func (*Federation) Close() error { return nil }

// Executor is never reached: Open above never returns a Federation to call it
// on. It exists so the registry compiles against one package rather than two.
func (*Federation) Executor() *sqldriver.Executor { return nil }
