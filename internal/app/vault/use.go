package vault

// Use names a definition that reads a secret.
type Use struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}
