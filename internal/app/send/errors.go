package send

import "errors"

var (
	// ErrForbidden means the principal may not send from this project.
	ErrForbidden = errors.New("send: not permitted")
	// ErrInvalid means the request does not describe a send.
	ErrInvalid = errors.New("send: not a send")
	// ErrRender means the report could not be produced. The recipients are
	// untouched: nothing is delivered before the document exists.
	ErrRender = errors.New("send: could not render")
)
