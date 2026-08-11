package definition

// DataSource is a connection cronos may read through.
//
// Held separately from datasets so credentials live in one place: a dataset
// names a source, and only the source knows the DSN. Rotating a password is
// then one edit rather than one per query.
type DataSource struct {
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// Driver selects the adapter: postgres, mysql, sqlite, object-store.
	Driver string `json:"driver" yaml:"driver"`
	// DSN is how to connect. ${secret:name} is resolved at load, never stored
	// expanded and never returned by the management API.
	DSN string `json:"dsn,omitempty" yaml:"dsn,omitempty"`

	// Object stores are addressed rather than connected to.
	URI         string `json:"uri,omitempty" yaml:"uri,omitempty"`
	Format      string `json:"format,omitempty" yaml:"format,omitempty"`
	Region      string `json:"region,omitempty" yaml:"region,omitempty"`
	Credentials string `json:"credentials,omitempty" yaml:"credentials,omitempty"`

	Pool   Pool   `json:"pool,omitzero" yaml:"pool,omitempty"`
	Limits Limits `json:"limits,omitzero" yaml:"limits,omitempty"`
}

// Federated reports whether reading this source means attaching it to a query
// engine rather than connecting to it directly.
func (d DataSource) Federated() bool { return d.Driver == "object-store" }
