package burst

import "errors"

var (
	// ErrNoRecipients means the `over` dataset returned nothing.
	//
	// An error and not a quiet success. A burst that delivered zero documents
	// looks identical to one that was never scheduled, and the first person to
	// notice is the customer who did not receive their invoice.
	ErrNoRecipients = errors.New("burst: no recipients")
	// ErrBind means a binding referred to a column the row does not have.
	ErrBind = errors.New("burst: cannot bind")
)
