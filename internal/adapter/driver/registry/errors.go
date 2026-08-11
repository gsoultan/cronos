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
