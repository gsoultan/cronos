package query

import "strconv"

// Dollar numbers arguments: $1, $2. Postgres, DuckDB, ClickHouse.
type Dollar struct{}

func (Dollar) At(n int) string { return "$" + strconv.Itoa(n) }
