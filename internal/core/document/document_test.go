package document_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/document"
)

/*
What a paginated document is, before Typst sees it.

The package had no tests, and one of the checks in it is the only thing
standing between a burst and five thousand confidently wrong statements: a row
with a missing cell does not fail to typeset. It shifts every cell after it one
column left, so an amount lands under "Status" and the document renders
perfectly. Nothing downstream can detect that, which is why it is caught here
and why the ragged-row case is the one worth being thorough about.

The rest is an allow-list and a bound, both of which exist so a typo is a
validation error rather than a failed render at recipient 3,000 of 5,000.
*/

func doc() document.Document {
	return document.Document{
		Title: "Billing summary",
		Columns: []document.Column{
			{Field: "id"}, {Field: "customer"}, {Field: "total"},
		},
		Groups: []document.Group{{
			Label: "August",
			Rows:  [][]string{{"1", "Acme", "12.00"}, {"2", "Globex", "8.50"}},
		}},
	}
}

func TestAWellFormedDocumentValidates(t *testing.T) {
	if err := doc().Validate(); err != nil {
		t.Fatalf("a good document was refused: %v", err)
	}
}

/*
The check that earns the package its tests.

A short row and a long one are both caught, and the message names the group,
the row and both counts — because the author is looking at a definition, not at
this struct, and "row 3 has 2 cells, want 3" is what points them at the query.
*/
func TestARaggedRowIsRefusedRatherThanTypeset(t *testing.T) {
	for _, c := range []struct {
		name string
		row  []string
	}{
		{"a cell short", []string{"3", "Initech"}},
		{"a cell over", []string{"3", "Initech", "1.00", "extra"}},
		{"empty", []string{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := doc()
			d.Groups[0].Rows = append(d.Groups[0].Rows, c.row)

			err := d.Validate()
			if !errors.Is(err, document.ErrInvalid) {
				t.Fatalf("got %v, want ErrInvalid", err)
			}
			// Named well enough to find, because the failure is silent
			// otherwise and the author has a query to fix.
			if !strings.Contains(err.Error(), "August") {
				t.Errorf("the message should name the group: %v", err)
			}
		})
	}
}

func TestADocumentNeedsColumnsAndGroups(t *testing.T) {
	noColumns := doc()
	noColumns.Columns = nil
	if err := noColumns.Validate(); !errors.Is(err, document.ErrInvalid) {
		t.Errorf("a document with no columns: got %v, want ErrInvalid", err)
	}

	noGroups := doc()
	noGroups.Groups = nil
	if err := noGroups.Validate(); !errors.Is(err, document.ErrInvalid) {
		t.Errorf("a document with no groups: got %v, want ErrInvalid", err)
	}
}

// A group with no label is a section heading nobody wrote, and it is also what
// the ragged-row message uses to say where the problem is.
func TestAGroupNeedsALabel(t *testing.T) {
	d := doc()
	d.Groups[0].Label = ""

	if err := d.Validate(); !errors.Is(err, document.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

// A subtotal over a column that is not in the document is a number computed
// from nothing, which Typst would render as zero.
func TestASubtotalOverAnUnknownColumnIsRefused(t *testing.T) {
	d := doc()
	d.Groups[0].Subtotal = map[string]string{"vat": "1.00"}

	err := d.Validate()
	if !errors.Is(err, document.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "vat") {
		t.Errorf("the message should name the column: %v", err)
	}

	// And one over a real column is fine.
	d.Groups[0].Subtotal = map[string]string{"total": "20.50"}
	if err := d.Validate(); err != nil {
		t.Errorf("a subtotal over a real column was refused: %v", err)
	}
}

/* -- the page ------------------------------------------------------------- */

// An allow-list, so a typo is a validation error and not a failed render
// halfway through a five-thousand-recipient burst.
func TestOnlyPapersAReportIsEverPrintedOnAreAccepted(t *testing.T) {
	for size := range document.PaperSizes {
		d := doc()
		d.Page.Size = size
		if err := d.Validate(); err != nil {
			t.Errorf("paper %q was refused: %v", size, err)
		}
	}

	for _, size := range []string{"a2", "A4", "letter", "foolscap"} {
		d := doc()
		d.Page.Size = size
		if err := d.Validate(); !errors.Is(err, document.ErrInvalid) {
			t.Errorf("paper %q: got %v, want ErrInvalid", size, err)
		}
	}
}

// Empty means the default, which is what keeps a definition that says nothing
// about paper from being a definition that will not publish.
func TestAnUnsetPageIsTheDefaultRatherThanAnError(t *testing.T) {
	if err := doc().Validate(); err != nil {
		t.Fatalf("a document that says nothing about its page was refused: %v", err)
	}
}

func TestOrientationIsPortraitLandscapeOrUnset(t *testing.T) {
	for _, o := range []string{"", "portrait", "landscape"} {
		d := doc()
		d.Page.Orientation = o
		if err := d.Validate(); err != nil {
			t.Errorf("orientation %q was refused: %v", o, err)
		}
	}

	for _, o := range []string{"sideways", "Portrait", "landscape "} {
		d := doc()
		d.Page.Orientation = o
		if err := d.Validate(); !errors.Is(err, document.ErrInvalid) {
			t.Errorf("orientation %q: got %v, want ErrInvalid", o, err)
		}
	}
}

/*
The margin is bounded because Typst's own error for an over-wide one names an
internal layout constraint rather than the number the author typed.

74mm is half of A5's short edge — the largest margin that still leaves a body.
*/
func TestTheMarginIsBoundedAtSomethingThatStillLeavesABody(t *testing.T) {
	for _, c := range []struct {
		mm    float64
		valid bool
	}{
		{0, true}, // zero means the default of 18
		{18, true},
		{74, true}, // the boundary is inclusive
		{74.1, false},
		{200, false},
		{-1, false},
	} {
		d := doc()
		d.Page.MarginMM = c.mm

		err := d.Validate()
		if c.valid && err != nil {
			t.Errorf("margin %gmm was refused: %v", c.mm, err)
		}
		if !c.valid && !errors.Is(err, document.ErrInvalid) {
			t.Errorf("margin %gmm: got %v, want ErrInvalid", c.mm, err)
		}
	}
}
