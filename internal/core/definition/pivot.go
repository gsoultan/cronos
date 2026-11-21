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
	// Sparkline adds a column beside each row's name that draws each row across the
	// top as a small line: twelve months of a line of the P&L read as a
	// shape before anyone reads its figures. Only across dates, because a
	// line through categories in alphabetical order is a shape of nothing.
	Sparkline bool `json:"sparkline,omitempty" yaml:"sparkline,omitempty"`
	// Shade colours each cell by where its figure falls among the others, on
	// the ramp a heatmap uses, so where the large cells are is seen before
	// any is read. The totals are left plain: they are a different size of
	// thing, and would take the darkest shade from every cell.
	Shade bool `json:"shade,omitempty" yaml:"shade,omitempty"`
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
	if p.Sparkline && p.Columns.Grain == "" {
		return fmt.Errorf("%w: %s pivot %d draws a sparkline across %q, which is not a date "+
			"with a grain — a line through categories draws the order they sort in",
			ErrInvalid, output, i, p.Columns.Field)
	}
	for _, d := range []DimensionRef{p.Rows, p.Columns} {
		if d.Grain != "" && !timeGrains[d.Grain] {
			return fmt.Errorf("%w: %s pivot %d buckets %q by %q, want day, week, month, quarter or year",
				ErrInvalid, output, i, d.Field, d.Grain)
		}
	}
	return nil
}
