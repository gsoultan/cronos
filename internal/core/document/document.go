package document

import "fmt"

// Document is one PDF: a title, the pages' furniture, and the groups that
// become its sections.
//
// Every cell in it is already a string. Formatting money, dates and locales is
// the engine's job, and doing it here rather than in the template keeps
// rounding in one place — a template that formats currency is a second
// implementation of the rules that decided what to bill.
type Document struct {
	Title   string   `json:"title"`
	Period  string   `json:"period"`
	Org     Org      `json:"org"`
	Page    Page     `json:"page"`
	Columns []Column `json:"columns"`
	Groups  []Group  `json:"groups"`
	// Charts are drawn above the table, in the order the layout listed them.
	//
	// Charts used to be dropped from a paginated output entirely: a report
	// that had them rendered a PDF that simply did not, with nothing saying
	// so, and the format documentation claimed they came out as images.
	Charts []Chart `json:"charts,omitempty"`
}

// Validate reports what would otherwise become a wrong document rather than a
// failed one.
//
// The ragged-row check is the one that earns its place. A row with a missing
// cell does not fail to typeset — it shifts every cell after it one column
// left, so an amount lands under "Status" and the statement is confidently
// wrong. Nothing downstream can detect that; it has to be caught here.
func (d Document) Validate() error {
	// A document of charts and no table is a legitimate paginated report — a
	// dashboard on paper. Columns and groups are required only when there is
	// nothing else on the page, because a document with neither is one that
	// renders blank.
	if len(d.Charts) == 0 {
		if len(d.Columns) == 0 {
			return fmt.Errorf("%w: no columns", ErrInvalid)
		}
		if len(d.Groups) == 0 {
			return fmt.Errorf("%w: no groups", ErrInvalid)
		}
	}
	if err := d.Page.validate(); err != nil {
		return err
	}
	for _, c := range d.Charts {
		if err := c.validate(); err != nil {
			return err
		}
	}
	return d.validateRows()
}

func (d Document) validateRows() error {
	want := len(d.Columns)
	for gi, g := range d.Groups {
		if g.Label == "" {
			return fmt.Errorf("%w: group %d has no label", ErrInvalid, gi)
		}
		for ri, row := range g.Rows {
			if len(row) != want {
				return fmt.Errorf("%w: group %q row %d has %d cells, want %d",
					ErrInvalid, g.Label, ri, len(row), want)
			}
		}
		for field := range g.Subtotal {
			if !d.hasColumn(field) {
				return fmt.Errorf("%w: group %q subtotals unknown column %q",
					ErrInvalid, g.Label, field)
			}
		}
	}
	return nil
}

func (d Document) hasColumn(field string) bool {
	for _, c := range d.Columns {
		if c.Field == field {
			return true
		}
	}
	return false
}
