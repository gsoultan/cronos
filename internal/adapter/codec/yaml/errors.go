package yaml

import "errors"

// ErrDecode marks a document the loader cannot read: wrong apiVersion,
// unknown kind, or a spec that does not match it.
//
// Distinct from definition.ErrInvalid, which is a document that parsed and
// then failed its rules. An author fixing "this is not a Dataset" is doing
// something different from one fixing "this Dataset has no query".
var ErrDecode = errors.New("yaml: cannot decode")
