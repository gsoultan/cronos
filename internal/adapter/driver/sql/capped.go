package sql

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/app/run"
)

// ErrTooManyRows is what a query that will not fit returns.
//
// An error and never a truncation. A report that quietly stopped at a million
// rows is a wrong answer presented as a right one, and the person reading it
// has no way to tell — whereas a refusal names the dataset and the cap, and
// the fix is a narrower query or a higher limit.
var ErrTooManyRows = fmt.Errorf("sql: too many rows")

// capped stops a result set from exceeding the source's row limit.
//
// AGENTS.md: "Every datasource carries a statement timeout and a row cap. No
// unbounded query." The timeout bounds what the database spends; this bounds
// what cronos accepts back, which is the half that decides whether a report
// over somebody else's ten-million-row table takes the server down.
type capped struct {
	run.Rows
	limit int
	read  int
	err   error
}

func (c *capped) Next() bool {
	if c.err != nil {
		return false
	}
	if c.read >= c.limit {
		c.err = fmt.Errorf("%w: more than %d rows", ErrTooManyRows, c.limit)
		return false
	}
	if !c.Rows.Next() {
		return false
	}
	c.read++
	return true
}

// Err reports the cap first.
//
// A caller that only checks Err — which is the correct way to read a result
// set — must not see a clean finish for a query that was cut short.
func (c *capped) Err() error {
	if c.err != nil {
		return c.err
	}
	return c.Rows.Err()
}
