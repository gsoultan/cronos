package document

import "fmt"

// Page is the paper the document is typeset onto.
//
// The margin is a number and not the "20mm" the definition format writes,
// because the template would otherwise have to parse it — and the only tool
// Typst offers for that is `eval`, which on a definition-supplied string is
// arbitrary code execution in the typesetter. Parsing happens in Go, where a
// bad unit is an error message rather than a capability.
type Page struct {
	// Size is a Typst paper name. Empty means a4. Validated against
	// PaperSizes before it reaches the template.
	Size string `json:"size"`
	// Orientation is "portrait" or "landscape". Empty means portrait.
	Orientation string `json:"orientation"`
	// MarginMM applies to all four sides. Zero means 18.
	MarginMM float64 `json:"marginMm"`
}

// PaperSizes are the papers a report definition may name. Typst knows many
// more; these are the ones an invoice statement is ever printed on, and an
// allow-list turns a typo into a validation error instead of a failed render
// halfway through a 5,000-recipient burst.
var PaperSizes = map[string]bool{
	"a4": true, "a5": true, "a3": true,
	"us-letter": true, "us-legal": true, "us-tabloid": true,
}

// maxMarginMM is half of A5's short edge. Anything larger leaves no body, and
// Typst's error for that names an internal layout constraint rather than the
// margin the author actually typed.
const maxMarginMM = 74

func (p Page) validate() error {
	if p.Size != "" && !PaperSizes[p.Size] {
		return fmt.Errorf("%w: unknown paper size %q", ErrInvalid, p.Size)
	}
	switch p.Orientation {
	case "", "portrait", "landscape":
	default:
		return fmt.Errorf("%w: orientation %q is not portrait or landscape",
			ErrInvalid, p.Orientation)
	}
	if p.MarginMM < 0 || p.MarginMM > maxMarginMM {
		return fmt.Errorf("%w: margin %gmm is outside 0–%dmm",
			ErrInvalid, p.MarginMM, maxMarginMM)
	}
	return nil
}
