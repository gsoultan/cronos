package query_test

import (
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/query"
)

/*
SQL Server, and the two things about it that are not like the others.

Its placeholder is named rather than positional or bare, and its date truncation
has to work on versions that predate the function everybody reaches for. Both
are the sort of difference that produces a statement which compiles and is
wrong, or one that fails four years into a deployment's life.
*/

func TestArgumentsAreNamedNotPositional(t *testing.T) {
	d := query.SQLServer{}

	// `?` reaches the server as a literal question mark and the statement fails
	// to compile, with a message about syntax several lines from the cause.
	for n, want := range map[int]string{1: "@p1", 2: "@p2", 17: "@p17"} {
		if got := d.At(n); got != want {
			t.Fatalf("argument %d is %q", n, got)
		}
	}
}

/*
Truncation uses arithmetic that has worked since 2008.

DATETRUNC is 2022 and later. A reporting tool meets a great many 2016 and 2019
servers — they are what sits behind an ERP — and on those DATETRUNC fails with
"not a recognized built-in function name", which a deployment discovers at six
in the morning on the first of the month.
*/
func TestBucketingAvoidsFunctionsOlderServersLack(t *testing.T) {
	d := query.SQLServer{}

	for _, grain := range []string{"day", "month", "quarter", "year"} {
		got, err := d.Bucket(grain, "o.placed_at")
		if err != nil {
			t.Fatalf("%s: %v", grain, err)
		}
		if strings.Contains(strings.ToUpper(got), "DATETRUNC") {
			t.Fatalf("%s uses DATETRUNC, which is 2022 and later: %s", grain, got)
		}
		if !strings.Contains(got, "DATEADD") || !strings.Contains(got, "DATEDIFF") {
			t.Fatalf("%s is %q", grain, got)
		}
		// The grain has to reach both halves, or the statement rounds by one
		// unit and adds back another — which produces dates, silently wrong.
		if strings.Count(got, grain) != 2 {
			t.Fatalf("%s appears %d times in %q", grain, strings.Count(got, grain), got)
		}
	}
}

/*
Weekly is refused rather than approximated.

DATEDIFF(week, …) counts Sunday boundaries whatever SET DATEFIRST says, so the
same report bucketed weekly would give one answer here and another on Postgres,
where a week starts on Monday. A chart that is quietly a day out is not read as
an error by anybody.
*/
func TestWeeklyIsRefusedRatherThanBeingADayOut(t *testing.T) {
	if _, err := (query.SQLServer{}).Bucket("week", "o.placed_at"); err == nil {
		t.Fatal("sqlserver bucketed by week, which disagrees with every other dialect")
	}
	// And it is refused as an unsupported combination, not as a bad grain:
	// "week" is a grain, it is this dialect that cannot do it.
	_, err := (query.SQLServer{}).Bucket("week", "x")
	if !strings.Contains(err.Error(), "week") {
		t.Fatalf("the message does not name the grain: %v", err)
	}
}

// A grain nobody defined is refused before it can become SQL text. This is the
// one place a definition's value is interpolated rather than bound.
func TestAnUnknownGrainNeverReachesTheStatement(t *testing.T) {
	for _, bad := range []string{"", "fortnight", "day'); DROP TABLE orders--"} {
		if _, err := (query.SQLServer{}).Bucket(bad, "x"); err == nil {
			t.Fatalf("%q was accepted as a grain", bad)
		}
	}
}
