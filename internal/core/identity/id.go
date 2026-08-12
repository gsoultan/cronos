package identity

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID names a person.
//
// Its own shape rather than a run's. The CLI had been minting user ids with
// the run-record generator, so every person in the store was called
// `run_1786…` — which works, and is the sort of thing somebody reads in an
// audit trail at two in the morning and loses ten minutes to.
//
// Random rather than sequential: a sequential one tells whoever holds it how
// many people this deployment has.
func NewID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice, and an account that could not
		// be named must not be an account that was not created.
		return "usr_unnamed"
	}
	return "usr_" + hex.EncodeToString(b[:])
}

// Acceptable reports whether a password may be used.
//
// Exported so the API can refuse a weak one with the same sentence the CLI
// does, before it reaches a hash — rather than each caller inventing its own
// rule and the two disagreeing about what is short.
func Acceptable(password string) error {
	return check(password)
}
