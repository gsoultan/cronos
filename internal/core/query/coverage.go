package query

// Coverage records which shared filters reached this dataset.
//
// It exists so a block can say "not affected by Period" in the interface. The
// report format promises that, and it can only be kept if compilation reports
// it — a filter that silently applies to some blocks and not others is worse
// than one that admits it, and nothing downstream can work out which happened.
type Coverage struct {
	// Applied names the filters that narrowed this dataset, in report order.
	Applied []string
	// Ignored names the filters with no binding for this dataset.
	Ignored []string
}

// Complete reports whether every filter reached this dataset.
func (c Coverage) Complete() bool { return len(c.Ignored) == 0 }
