package definition

import "fmt"

// validateTimeline checks a timeline: what each bar is, and the two dates it
// runs between. It reads no measure — a bar's length is its dates — and its
// dates are whole days, so neither takes a grain.
func (b Block) validateTimeline(output string, i int) error {
	switch {
	case b.X.Field == "":
		return fmt.Errorf("%w: %s timeline %d needs x to name what each bar is", ErrInvalid, output, i)
	case b.Start.Field == "" || b.End.Field == "":
		return fmt.Errorf("%w: %s timeline %d needs a start and an end, the dates each bar "+
			"runs between", ErrInvalid, output, i)
	case b.Y.Field != "":
		return fmt.Errorf("%w: %s timeline %d sets y, which a timeline does not read — "+
			"a bar is as long as its dates", ErrInvalid, output, i)
	case b.X.Grain != "" || b.Start.Grain != "" || b.End.Grain != "":
		return fmt.Errorf("%w: %s timeline %d sets a grain, and its bars run day to day",
			ErrInvalid, output, i)
	}
	return nil
}
