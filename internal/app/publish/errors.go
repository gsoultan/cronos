package publish

import "errors"

var (
	// ErrForbidden means the principal may not change definitions here.
	ErrForbidden = errors.New("publish: not permitted")
	// ErrUnsupported means the document is a kind this build does not store.
	ErrUnsupported = errors.New("publish: unsupported kind")
	// ErrNotFound means there is no such definition to read or remove.
	ErrNotFound = errors.New("publish: no such definition")
)
