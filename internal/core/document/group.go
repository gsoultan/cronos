package document

// Group is one section of the document — in a burst, one recipient.
//
// A group is the unit the page counter restarts against: the footer numbers a
// recipient's own statement, so page 2 of 2 means theirs and not the run's.
type Group struct {
	Label   string   `json:"label"`
	Address []string `json:"address,omitempty"`
	Meta    []Entry  `json:"meta,omitempty"`
	// Rows are pre-formatted cells, in Columns order.
	Rows [][]string `json:"rows"`
	// Subtotal is keyed by Column.Field. Computed upstream, never here.
	Subtotal map[string]string `json:"subtotal,omitempty"`
}
