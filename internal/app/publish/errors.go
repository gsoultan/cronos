package publish

import "errors"

var (
	// ErrForbidden means the principal may not change definitions here.
	ErrForbidden = errors.New("publish: not permitted")
	// ErrUnsupported means the document is a kind this build does not store.
	ErrUnsupported = errors.New("publish: unsupported kind")
	// ErrNotFound means there is no such definition to read or remove.
	ErrNotFound = errors.New("publish: no such definition")
	// ErrInUse means something still points at it, so removing it would break
	// whatever that is.
	ErrInUse = errors.New("publish: still in use")
	// ErrScopedBySchedule means a schedule reads a dataset with row-level
	// security, which would deliver empty documents to everybody.
	ErrScopedBySchedule = errors.New("publish: schedule reads a row-scoped dataset")
	// ErrStale means somebody else saved this definition since the caller read
	// it, so storing theirs would quietly discard the other edit.
	ErrStale = errors.New("publish: changed since you read it")
	// ErrNoSuchChannel means a schedule delivers through a channel this
	// deployment has not configured, which the burst would only discover at
	// the hour the schedule fires.
	ErrNoSuchChannel = errors.New("publish: no such delivery channel")
)
