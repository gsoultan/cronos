package query

import (
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// DateLayout is the only date a caller may write. One format, not a list of
// tolerated ones: 03/04/2026 is the fourth of March to half the world and the
// third of April to the other half, and a report engine that guesses will be
// wrong about a month's revenue without anyone noticing.
const DateLayout = "2006-01-02"

// relative names the dates a definition may write as a default.
//
// Kept to a handful and resolved here rather than in a general expression
// language. "today" in a default is a scheduling convenience; a dataset that
// needs real date arithmetic should take the dates as parameters and let the
// schedule compute them, where the result is visible in the run record.
var relative = map[string]func(time.Time) time.Time{
	"today":     func(t time.Time) time.Time { return t },
	"yesterday": func(t time.Time) time.Time { return t.AddDate(0, 0, -1) },
	"tomorrow":  func(t time.Time) time.Time { return t.AddDate(0, 0, 1) },
}

func asDate(p definition.Param, raw any, now func() time.Time) (any, error) {
	// YAML resolves an unquoted 2026-07-01 to a timestamp, so a definition's
	// own default arrives here already parsed. Refusing it would mean every
	// author has to know to quote a date, and the ones who do not would get
	// "wants a date as YYYY-MM-DD" about a date.
	if t, isTime := raw.(time.Time); isTime {
		return t.Format(DateLayout), nil
	}
	s, ok := raw.(string)
	if !ok {
		return nil, typeErr(p, "a date as YYYY-MM-DD")
	}
	if shift, isRelative := relative[s]; isRelative {
		return shift(now()).Format(DateLayout), nil
	}
	if _, err := time.Parse(DateLayout, s); err != nil {
		return nil, fmt.Errorf("%w: %q wants a date as YYYY-MM-DD", ErrBadArgument, p.Name)
	}
	return s, nil
}
