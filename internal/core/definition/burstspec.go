package definition

import "runtime"

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

/*
DefaultConcurrency keeps a typesetter busy without turning a burst into a load
test of the database behind it.

It was eight, on every machine. Measured on fifteen cores, eight workers
delivered 549 documents a second and sixteen delivered 671 — a fifth of the
throughput left unused — while on a two-core container eight is four processes
per core, each one a typesetter, which is the thrashing the number was there to
prevent. One constant cannot be both.

So it follows the machine, bounded at each end. The floor is four because a
worker spends most of its life waiting — on a query, on a typesetter, on a
delivery — so even one core wants several in flight. The ceiling is thirty-two
because the measurement flattens after the core count and a burst is not
supposed to be the only thing this process can do.

A schedule that knows better says a number, and this is not consulted.
*/
func DefaultConcurrency() int {
	workers := runtime.NumCPU()
	if workers < minConcurrency {
		return minConcurrency
	}
	if workers > maxConcurrency {
		return maxConcurrency
	}
	return workers
}

const (
	minConcurrency = 4
	maxConcurrency = 32
)

// Workers is the concurrency to actually use.
func (b BurstSpec) Workers() int {
	if b.Concurrency > 0 {
		return b.Concurrency
	}
	return DefaultConcurrency()
}

// OverSpec names the dataset whose rows are the recipients.
type OverSpec struct {
	Dataset string `json:"dataset" yaml:"dataset"`
}
