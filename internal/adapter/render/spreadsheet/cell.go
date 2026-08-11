package spreadsheet

// Cell is one value, and whether it is a number.
//
// The distinction is the whole point of the format. A recipient who wanted the
// figures as text would have taken the PDF.
type Cell struct {
	Text string
	// Number is set when the value is numeric, and Text is then only what the
	// engine formatted it as — the spreadsheet shows its own formatting.
	Number  float64
	Numeric bool
}

// Text makes a string cell.
func Str(s string) Cell { return Cell{Text: s} }

// Num makes a numeric cell.
func Num(v float64) Cell { return Cell{Number: v, Numeric: true} }
