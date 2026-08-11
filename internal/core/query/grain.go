package query

import "fmt"

// Grains are the periods a chart may bucket by.
//
// A closed list because it comes from a definition and every entry needs an
// implementation in each dialect. A grain nobody implemented is better as a
// validation error than as whatever the database did with the word.
var Grains = map[string]bool{
	"day": true, "week": true, "month": true, "quarter": true, "year": true,
}

func unsupportedGrain(dialect, grain string) error {
	if !Grains[grain] {
		return fmt.Errorf("%w: %q is not a grain — use day, week, month, quarter or year",
			ErrBadTemplate, grain)
	}
	return fmt.Errorf("%w: %s cannot bucket by %s", ErrBadTemplate, dialect, grain)
}
