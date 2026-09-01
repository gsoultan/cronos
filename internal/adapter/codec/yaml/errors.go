package yaml

import "errors"

// ErrDecode marks a document the loader cannot read: wrong apiVersion,
// unknown kind, or a spec that does not match it.
//
// Distinct from definition.ErrInvalid, which is a document that parsed and
// then failed its rules. An author fixing "this is not a Dataset" is doing
// something different from one fixing "this Dataset has no query".
var ErrDecode = errors.New("yaml: cannot decode")

// ErrEncode marks a definition that could not be written as a document.
//
// Separate from definition.ErrInvalid, which Encoder returns unwrapped when a
// caller hands it a definition the engine would refuse: "this dataset has no
// query" is the author's problem and reads better than an encoder wrapping it.
// This one is the encoder's own failure, and reaching it means a kind gained a
// field that will not marshal.
var ErrEncode = errors.New("yaml: cannot encode")
