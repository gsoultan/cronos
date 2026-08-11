package query

// Placeholder writes the marker a driver uses for the n-th bind argument.
//
// Dialects disagree — Postgres numbers them, MySQL does not — and that
// disagreement is the whole of the difference as far as this package is
// concerned. Everything else about a compiled statement is dialect-neutral,
// so this stays one method rather than growing into a dialect object.
type Placeholder interface {
	// At returns the placeholder for argument n, counting from 1.
	At(n int) string
}
