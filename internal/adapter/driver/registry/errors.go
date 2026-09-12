package registry

import "errors"

var (
	// ErrUnknownSource means a dataset names a datasource nobody defined.
	ErrUnknownSource = errors.New("registry: no such datasource")
	// ErrNoFederation means a dataset needs more than one source and this
	// build cannot join across them.
	ErrNoFederation = errors.New("registry: this build cannot federate")
	// ErrNoSources means a dataset names none at all, which validation should
	// have caught — repeated here because the alternative is a nil connection.
	ErrNoSources = errors.New("registry: dataset names no source")
)

// ErrBadMountName means a dataset calls a source something a query cannot use
// as a name, or calls two of them the same thing.
var ErrBadMountName = errors.New("registry: a source is not named for a query")

// ErrClosed means a query arrived after the registry was shut down.
//
// Refused rather than served: opening a federation here would attach somebody
// else's warehouse with nothing left to close it.
var ErrClosed = errors.New("registry: closed")
