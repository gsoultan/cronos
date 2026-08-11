package document

import "errors"

// ErrInvalid marks a document the renderer refuses rather than
// typesets. Separate from a compile failure on purpose: this one is the
// caller's bug and the message says which field, whereas a compile failure is
// the typesetter's report about source it was given.
var ErrInvalid = errors.New("document: invalid")
