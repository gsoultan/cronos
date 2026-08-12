package query

import (
	"fmt"
	"strconv"
)

/*
SQLServer names arguments and truncates dates by rounding a difference.

The placeholder is `@p1`, which is the only style the driver's positional
binding produces — a `?` reaches the server as a literal question mark and the
statement fails to compile with a message about incorrect syntax, several lines
away from the argument that caused it.
*/
type SQLServer struct{}

func (SQLServer) At(n int) string { return "@p" + strconv.Itoa(n) }

/*
Bucket truncates by counting periods from a fixed origin and adding them back.

`DATETRUNC` would be the obvious answer and is SQL Server 2022 and later. A
great many of the databases this will meet are 2016 and 2019 — they are what a
reporting tool finds behind an ERP — and on those it fails with "DATETRUNC is
not a recognized built-in function name", which is a deployment discovering at
six in the morning that its warehouse is four years older than this code
assumed.

`DATEADD(unit, DATEDIFF(unit, 0, expr), 0)` is the form that has worked since
2008. The zero is 1900-01-01, and the arithmetic is: how many whole units lie
between then and this date, added back to then.

Week is deliberately absent. `DATEDIFF(week, …)` counts Sunday boundaries
regardless of `SET DATEFIRST`, so the same report bucketed weekly gives one
answer here and a different one on Postgres, where a week begins on Monday.
Refusing is the honest outcome: a chart that is quietly a day out is not read as
an error by anybody.
*/
func (SQLServer) Bucket(grain, expr string) (string, error) {
	unit, ok := sqlServerUnits[grain]
	if !ok {
		return "", unsupportedGrain("sqlserver", grain)
	}
	// The grain is matched against the table above before it reaches the
	// statement, so this is the one place a definition value becomes SQL text
	// and it can only be one of four words.
	return fmt.Sprintf("DATEADD(%s, DATEDIFF(%s, 0, %s), 0)", unit, unit, expr), nil
}

var sqlServerUnits = map[string]string{
	"day":     "day",
	"month":   "month",
	"quarter": "quarter",
	"year":    "year",
}
