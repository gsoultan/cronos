package share

import "errors"

var (
	// ErrForbidden means the principal may not share or revoke here.
	ErrForbidden = errors.New("share: not permitted")
	// ErrInvalid means the request does not describe a share.
	ErrInvalid = errors.New("share: not a share")
	// ErrScoped means the report's rows belong to one customer at a time, so a
	// link to it would have to say which.
	ErrScoped = errors.New("share: this report's rows are scoped per customer")
	// ErrNotOpen means the link does not open: it never existed, it expired,
	// or somebody withdrew it. Deliberately one error for all three.
	ErrNotOpen = errors.New("share: this link does not open")
)
