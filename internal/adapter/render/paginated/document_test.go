package paginated

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name string
		doc  func(Document) Document
		says string
	}{
		{"a ragged row", func(d Document) Document {
			d.Groups[0].Rows[1] = []string{"INV-1", "2026-07-14"}
			return d
		}, "2 cells, want 4"},

		{"a subtotal on no column", func(d Document) Document {
			d.Groups[0].Subtotal = map[string]string{"profit": "€1.00"}
			return d
		}, `unknown column "profit"`},

		{"an unlabelled group", func(d Document) Document {
			d.Groups[0].Label = ""
			return d
		}, "no label"},

		{"no columns", func(d Document) Document {
			d.Columns = nil
			return d
		}, "no columns"},

		{"no groups", func(d Document) Document {
			d.Groups = nil
			return d
		}, "no groups"},

		{"a paper nobody prints on", func(d Document) Document {
			d.Page.Size = "a4-ish"
			return d
		}, `unknown paper size "a4-ish"`},

		{"a sideways orientation", func(d Document) Document {
			d.Page.Orientation = "sideways"
			return d
		}, "not portrait or landscape"},

		{"a margin with no page left", func(d Document) Document {
			d.Page.MarginMM = 200
			return d
		}, "outside 0–74mm"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.doc(fixture(3)).Validate()
			if !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("got %v, want ErrInvalidDocument", err)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("message %q does not say %q", err, c.says)
			}
		})
	}
}

func TestValidateAcceptsTheDefaults(t *testing.T) {
	d := fixture(2)
	d.Page = Page{} // every field optional
	if err := d.Validate(); err != nil {
		t.Errorf("an unconfigured page should typeset on A4: %v", err)
	}
}

// A group with no rows is a customer who was billed nothing this period. That
// is a real statement to send, not an error — the burst decides whether to
// send it, and the renderer should not make that choice by refusing.
func TestValidateAcceptsAnEmptyGroup(t *testing.T) {
	d := fixture(0)
	if err := d.Validate(); err != nil {
		t.Errorf("a nil-invoice statement is legitimate: %v", err)
	}
}
