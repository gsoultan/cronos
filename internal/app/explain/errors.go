package explain

import "errors"

var (
	// ErrInvalid is a request this package refuses before spending anything:
	// a block that is not in the view, a question too long to be one.
	ErrInvalid = errors.New("invalid explanation request")
	// ErrDeclined is the model choosing not to answer.
	ErrDeclined = errors.New("the model declined to answer")
)
