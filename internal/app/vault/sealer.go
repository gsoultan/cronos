package vault

// Sealer encrypts a value bound to where it belongs, and opens it again.
// secret.Sealer is the one there is.
type Sealer interface {
	Seal(value string, bound ...string) []byte
	Open(sealed []byte, bound ...string) (string, error)
	Stale(sealed []byte) bool
}
