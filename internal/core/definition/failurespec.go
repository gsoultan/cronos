package definition

// FailureSpec says what happens when a delivery does not work.
type FailureSpec struct {
	Retries int    `json:"retries,omitempty" yaml:"retries,omitempty"`
	Backoff string `json:"backoff,omitempty" yaml:"backoff,omitempty"`
	// Alert is where a human is told. A burst that silently delivers nothing
	// is the failure mode that costs a customer relationship, because the
	// first person to notice is the one who did not receive their invoice.
	Alert string `json:"alert,omitempty" yaml:"alert,omitempty"`
}
