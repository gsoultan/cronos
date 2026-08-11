package definition

import "fmt"

// drivers are the adapters this build can open a source with.
var drivers = map[string]bool{
	"postgres": true, "mysql": true, "sqlite": true, "duckdb": true, "object-store": true,
}

// formats are what an object store may hold.
var formats = map[string]bool{"parquet": true, "csv": true, "json": true}

// Validate reports every reason the datasource cannot be stored.
func (d DataSource) Validate() error {
	switch {
	case !slug.MatchString(d.Name):
		return fmt.Errorf("%w: name %q must be lowercase letters, digits and dashes", ErrInvalid, d.Name)
	case !drivers[d.Driver]:
		return fmt.Errorf("%w: source %q has driver %q, which this build cannot open",
			ErrInvalid, d.Name, d.Driver)
	}
	if d.Federated() {
		return d.validateObjectStore()
	}
	if d.DSN == "" {
		return fmt.Errorf("%w: source %q has no dsn", ErrInvalid, d.Name)
	}
	return d.validateLimits()
}

func (d DataSource) validateObjectStore() error {
	switch {
	case d.URI == "":
		return fmt.Errorf("%w: source %q has no uri", ErrInvalid, d.Name)
	case !formats[d.Format]:
		// Guessing from the extension is how a directory of .csv.gz gets read
		// as one enormous unparsed string.
		return fmt.Errorf("%w: source %q holds %q, want parquet, csv or json",
			ErrInvalid, d.Name, d.Format)
	}
	return d.validateLimits()
}

func (d DataSource) validateLimits() error {
	if d.Limits.MaxRows < 0 || d.Limits.StatementTimeout < 0 {
		return fmt.Errorf("%w: source %q has a negative limit", ErrInvalid, d.Name)
	}
	if d.Pool.MaxOpen < 0 {
		return fmt.Errorf("%w: source %q has a negative pool size", ErrInvalid, d.Name)
	}
	return nil
}
