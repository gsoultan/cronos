package run

import (
	"fmt"
	"time"
)

// bucketLayouts render a truncated date as a person reads it.
//
// The database returns the first instant of the period — "2026-07-01" for
// July — which is correct and unreadable. An axis labelled with twelve
// identical-looking dates is one nobody can scan.
var bucketLayouts = map[string]string{
	"day":   "2 Jan",
	"week":  "2 Jan",
	"month": "Jan 2006",
	"year":  "2006",
}

// bucketLabel formats a chart's x value for its grain.
//
// Falls back to the raw cell rather than guessing. A bucket that will not
// parse is a dimension that is not a date — grouping by status, say — and
// "Jan 2006"-ing a status is worse than printing it.
func bucketLabel(v any, grain string) string {
	t, ok := asTime(v)
	if !ok {
		return cell(v)
	}
	if grain == "quarter" {
		// No layout expresses a quarter, so it is spelled out.
		return fmt.Sprintf("Q%d %d", (int(t.Month())-1)/3+1, t.Year())
	}
	layout, known := bucketLayouts[grain]
	if !known {
		return cell(v)
	}
	if grain == "week" {
		return "w/c " + t.Format(layout)
	}
	return t.Format(layout)
}

// asTime coerces what a driver returned. Postgres hands back a time.Time from
// date_trunc; SQLite hands back the string strftime produced.
func asTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		return parseDate(t)
	case []byte:
		return parseDate(string(t))
	}
	return time.Time{}, false
}

func parseDate(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
