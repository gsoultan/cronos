package definition

import "fmt"

// Pivot is a crosstab: one dimension down the side, one across the top, and a
// measure folded into every cell where they meet.
//
// The measure is the block's `value`, as a stat's is, so a pivot names one
// number the same way the block that shows it as a headline does. Only the two
// axes are its own.
type Pivot struct {
	Rows    DimensionRef `json:"rows" yaml:"rows"`
	Columns DimensionRef `json:"columns" yaml:"columns"`
}

// validate reports why a pivot cannot be stored.
func (b Block) validatePivot(output string, i int) error {
	p := b.Pivot
	switch {
	case p == nil:
		return fmt.Errorf("%w: %s pivot %d has no pivot: — it needs rows and columns to cross",
			ErrInvalid, output, i)
	case p.Rows.Field == "" || p.Columns.Field == "":
		return fmt.Errorf("%w: %s pivot %d needs both rows and columns", ErrInvalid, output, i)
	case p.Rows == p.Columns:
		// Every cell off the diagonal would be empty: a list wearing a grid.
		return fmt.Errorf("%w: %s pivot %d crosses %q with itself", ErrInvalid, output, i, p.Rows.Field)
	case b.Value.Field == "":
		return fmt.Errorf("%w: %s pivot %d measures no field — set value", ErrInvalid, output, i)
	}
	for _, d := range []DimensionRef{p.Rows, p.Columns} {
		if d.Grain != "" && !timeGrains[d.Grain] {
			return fmt.Errorf("%w: %s pivot %d buckets %q by %q, want day, week, month, quarter or year",
				ErrInvalid, output, i, d.Field, d.Grain)
		}
	}
	return nil
}
