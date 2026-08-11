package definition

// BurstSpec fans one report out into a document per row.
//
// The row is the recipient. `over` names a dataset of them — customers,
// branches, cost centres — and `bind` maps each row's columns onto the
// report's parameters, so the same definition produces five thousand documents
// that each look like it was written for one person.
type BurstSpec struct {
	Over OverSpec `json:"over" yaml:"over"`
	// Bind maps a report parameter to a value drawn from the row, using
	// {{ .row.field }} or {{ .run.field }}.
	Bind map[string]string `json:"bind" yaml:"bind"`
	// Concurrency bounds how many documents render at once. Zero means
	// DefaultConcurrency.
	//
	// Bounded because the point of a burst is that it does not need a browser
	// farm: five thousand recipients rendered eight at a time is a steady
	// process, and five thousand at once is a machine that stops.
	Concurrency int `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
}

// DefaultConcurrency is a number that keeps a typesetter busy without turning
// a burst into a load test of the database behind it.
const DefaultConcurrency = 8

// Workers is the concurrency to actually use.
func (b BurstSpec) Workers() int {
	if b.Concurrency > 0 {
		return b.Concurrency
	}
	return DefaultConcurrency
}

// OverSpec names the dataset whose rows are the recipients.
type OverSpec struct {
	Dataset string `json:"dataset" yaml:"dataset"`
}
