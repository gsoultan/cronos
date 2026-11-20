package definition

import (
	"fmt"
	"slices"
)

// validateBars reports why a table's data bars cannot be drawn: a bar names
// a column the table lists, once.
//
// Whether the column is a number is the data's to say rather than the
// definition's. A cell that is not one draws no bar, which is what an
// empty measure should look like too.
func (b Block) validateBars(output string, i int) error {
	seen := map[string]bool{}
	for _, name := range b.Bars {
		switch {
		case !slices.Contains(b.Columns, name):
			return fmt.Errorf("%w: %s table %d draws bars in %q, which is not one of its columns",
				ErrInvalid, output, i, name)
		case seen[name]:
			return fmt.Errorf("%w: %s table %d draws bars in %q twice", ErrInvalid, output, i, name)
		}
		seen[name] = true
	}
	return nil
}
