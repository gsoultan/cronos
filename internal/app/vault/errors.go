package vault

import "errors"

var (
	// ErrForbidden means the caller may not see or change this project's
	// secrets. Editors may: they can already publish a datasource that sends a
	// secret anywhere they like, so refusing them the list protects nothing.
	ErrForbidden = errors.New("vault: not permitted")
	// ErrUnavailable means this deployment has no key to seal a secret with,
	// so it has nowhere safe to keep one.
	ErrUnavailable = errors.New("vault: this deployment cannot store secrets")
	// ErrInvalid means the name or the value cannot be stored as given.
	ErrInvalid = errors.New("vault: not a secret this can store")
	// ErrNotFound means there is no such secret stored in this project.
	ErrNotFound = errors.New("vault: no such secret")
	// ErrFull means the project holds as many secrets as it may.
	ErrFull = errors.New("vault: too many secrets")
)
