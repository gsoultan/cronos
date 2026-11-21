package definition

import "fmt"

// validateDumbbell checks a dumbbell: two metrics, the one each row runs from
// and the one it runs to, against each category of x.
func (b Block) validateDumbbell(output string, i int) error {
	switch {
	case b.Y.Field != "":
		return fmt.Errorf("%w: %s dumbbell %d sets y — a dumbbell reads two metrics, the one "+
			"each row runs from and the one it runs to", ErrInvalid, output, i)
	case len(b.Metrics) != 2:
		return fmt.Errorf("%w: %s dumbbell %d lists %d metrics — want two, the one each row "+
			"runs from and the one it runs to", ErrInvalid, output, i, len(b.Metrics))
	case b.X.Field == "":
		return fmt.Errorf("%w: %s dumbbell %d needs x to say what each row is", ErrInvalid, output, i)
	}
	for _, m := range b.Metrics {
		if err := m.validate(output, i, false); err != nil {
			return err
		}
	}
	return nil
}
