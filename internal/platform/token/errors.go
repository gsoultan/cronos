package token

import "errors"

// ErrInvalid covers every reason a token is not accepted.
//
// One error, on purpose. Distinguishing "bad signature" from "expired" for the
// caller turns verification into an oracle: an attacker learns which half of
// their forgery worked. The specific reason is logged, never returned.
var ErrInvalid = errors.New("token: invalid")

// ErrWeakKey is a configuration failure, not a request failure — it is raised
// when the process starts, where an operator can see it.
var ErrWeakKey = errors.New("token: signing key is too short")
