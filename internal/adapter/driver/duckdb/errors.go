package duckdb

import "errors"

/*
ErrNotBuilt is what federation is without the build tag.

A sentinel and not a missing package: the registry imports this package in
every build and asks for a federation whenever a dataset needs one, so the
answer in a pure-Go build has to be an error somebody can read — and one the
registry can recognise, to say `-tags duckdb` rather than to report a mount
that failed for reasons nobody can act on.
*/
var ErrNotBuilt = errors.New(
	"duckdb: this build has no federation — rebuild with -tags duckdb")
