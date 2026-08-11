package query

import (
	"fmt"
	"strconv"
)

// Postgres numbers arguments and truncates dates natively. Also DuckDB and
// ClickHouse, which follow it closely enough here.
type Postgres struct{}

func (Postgres) At(n int) string { return "$" + strconv.Itoa(n) }

func (Postgres) Bucket(grain, expr string) (string, error) {
	if !Grains[grain] {
		return "", unsupportedGrain("postgres", grain)
	}
	// The grain is checked against the list above before it reaches the
	// statement, so this is the one place a definition value becomes SQL text
	// and it can only be one of five words.
	return fmt.Sprintf("date_trunc('%s', %s)", grain, expr), nil
}
