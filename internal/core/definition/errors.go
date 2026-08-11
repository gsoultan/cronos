package definition

import "errors"

// ErrInvalid marks a definition the engine refuses to store.
//
// Validation happens on save. A dataset that cannot be bound is a dataset that
// fails at 6am in the middle of a burst, when the person who wrote it is
// asleep and the only evidence is a stack trace in a delivery log.
var ErrInvalid = errors.New("definition: invalid")
