package definition

import "fmt"

// Trend is a stat tile's number over time: its measure in each period of a
// date, drawn as a line under the figure, and the last period set against
// the one before it as the change the tile reports.
type Trend struct {
	// Field is the date the periods are read from, and Grain how long each
	// is: day, week, month, quarter or year.
	Field string `json:"field" yaml:"field"`
	Grain string `json:"grain" yaml:"grain"`
	// Better is which way is good news: higher, the default, or lower — for
	// a measure like days to pay, whose rise is the bad kind of change.
	Better string `json:"better,omitempty" yaml:"better,omitempty"`
}

// Lower reports whether a fall is the good news.
func (t Trend) Lower() bool { return t.Better == "lower" }

// validate reports why a stat's trend cannot be stored.
func (t Trend) validate(output string, i int) error {
	switch {
	case t.Field == "":
		return fmt.Errorf("%w: %s stat %d has a trend over no field — it needs the date "+
			"its periods are read from", ErrInvalid, output, i)
	case !timeGrains[t.Grain]:
		return fmt.Errorf("%w: %s stat %d trends by %q, want day, week, month, quarter or year",
			ErrInvalid, output, i, t.Grain)
	case t.Better != "" && t.Better != "higher" && t.Better != "lower":
		return fmt.Errorf("%w: %s stat %d says better is %q, want higher or lower",
			ErrInvalid, output, i, t.Better)
	}
	return nil
}
