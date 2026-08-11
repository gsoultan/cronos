//go:build !duckdb

package duckdb

import (
	"context"
	"errors"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// ErrNotBuilt is what federation is without the build tag.
//
// A clear error and not a missing package: a deployment that wants federation
// should learn it needs `-tags duckdb` from a message, not from a compiler.
var ErrNotBuilt = errors.New(
	"duckdb: this build has no federation — rebuild with -tags duckdb")

// Federation is the shape the tagged build provides.
type Federation struct{}

// Open always fails in an untagged build.
func Open(context.Context, map[string]definition.DataSource) (*Federation, error) {
	return nil, ErrNotBuilt
}

// Close is a no-op.
func (*Federation) Close() error { return nil }
