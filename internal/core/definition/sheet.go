package definition

import "fmt"

// Sheet is one tab of a spreadsheet output.
type Sheet struct {
	Name    string   `json:"name" yaml:"name"`
	Columns []string `json:"columns" yaml:"columns"`
	// FreezeHeader and AutoFilter are the two things every recipient does by
	// hand the moment they open the file, so the export does them.
	FreezeHeader bool `json:"freezeHeader,omitempty" yaml:"freezeHeader,omitempty"`
	AutoFilter   bool `json:"autoFilter,omitempty" yaml:"autoFilter,omitempty"`

	// Pivot makes the tab a crosstab of Value rather than a list of Columns:
	// the same one a pivot block draws, with the numbers left as numbers.
	Pivot *Pivot     `json:"pivot,omitempty" yaml:"pivot,omitempty"`
	Value MeasureRef `json:"value,omitzero" yaml:"value,omitempty"`
}

// validate reports why a tab cannot be written.
func (s Sheet) validate(output string, i int) error {
	switch {
	case s.Pivot != nil && len(s.Columns) > 0:
		return fmt.Errorf("%w: %s sheet %d lists columns and a pivot — a tab is one or the other",
			ErrInvalid, output, i)
	case s.Pivot != nil:
		return Block{Kind: PivotBlock, Value: s.Value, Pivot: s.Pivot}.validatePivot(output, i)
	case s.Value.Field != "":
		return fmt.Errorf("%w: %s sheet %d sets a value, which only a pivot reads", ErrInvalid, output, i)
	}
	return nil
}
