package query

import "fmt"

// SQLite marks arguments positionally and has no date_trunc, so grains become
// strftime formats.
type SQLite struct{}

func (SQLite) At(int) string { return "?" }

// formats are the strftime patterns that land each grain on its first day.
//
// Week and quarter are absent rather than approximated. SQLite can express
// them, but only with arithmetic whose week-start and quarter-start
// conventions differ from Postgres's — and a chart that buckets differently
// depending on which database answered is worse than one that refuses.
var formats = map[string]string{
	"day":   "%Y-%m-%d",
	"month": "%Y-%m-01",
	"year":  "%Y-01-01",
}

func (SQLite) Bucket(grain, expr string) (string, error) {
	f, ok := formats[grain]
	if !ok {
		return "", unsupportedGrain("sqlite", grain)
	}
	return fmt.Sprintf("strftime('%s', %s)", f, expr), nil
}
