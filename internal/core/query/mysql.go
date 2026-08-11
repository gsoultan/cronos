package query

import "fmt"

// MySQL marks arguments positionally and truncates with DATE_FORMAT.
type MySQL struct{}

func (MySQL) At(int) string { return "?" }

func (MySQL) Bucket(grain, expr string) (string, error) {
	f, ok := formats[grain]
	if !ok {
		return "", unsupportedGrain("mysql", grain)
	}
	return fmt.Sprintf("DATE_FORMAT(%s, '%s')", expr, f), nil
}
