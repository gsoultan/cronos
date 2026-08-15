package schedule

import (
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
The two days a year a clock is not a clock.

Berlin puts the clocks forward on 29 March 2026 (02:00 becomes 03:00) and back
on 25 October (03:00 becomes 02:00). A schedule that names a time inside either
window is the interesting one, and "half past two" is a time somebody picks for
exactly the reason it is dangerous — nobody is using the system then.

Going back is the one that costs money. The hour happens twice, so the firing
happens twice: eight hundred customers were sent their statement, and an hour
later the same schedule fired again and sent them all a second copy. There is
no partial-failure story here and nothing to reconcile against, because both
runs succeeded. It is the same harm live-resume.sh exists to prevent, arriving
from a direction nothing was watching.
*/

func planFor(t *testing.T, expr, tz string) Plan {
	t.Helper()
	p, err := Parse(definition.Schedule{Name: "statements", Cron: expr, Timezone: tz})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// firings walks the schedule the way the service does — fire, then re-arm from
// what fired and when it finished.
func firings(p Plan, from time.Time, n int) []time.Time {
	out := make([]time.Time, 0, n)
	at := p.Next(from)
	for range n {
		out = append(out, at)
		// The service re-arms once the run is done, so "now" is a little after
		// the firing. A minute is generous for a report and is the case that
		// matters: a slower run is further from the boundary, not closer.
		at = p.NextAfter(at, at.Add(time.Minute))
	}
	return out
}

func TestAScheduleDoesNotFireTwiceWhenTheClocksGoBack(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no timezone database on this host")
	}

	// The 25th of October 2026, from two days out so the ordinary days either
	// side are in the answer too.
	p := planFor(t, "30 2 * * *", "Europe/Berlin")
	got := firings(p, time.Date(2026, 10, 23, 12, 0, 0, 0, berlin), 4)

	want := []string{
		"Sat 24 Oct 02:30 CEST",
		"Sun 25 Oct 02:30 CEST", // the first pass through the repeated hour
		"Mon 26 Oct 02:30 CET",  // and not 25 Oct 02:30 CET, an hour later
		"Tue 27 Oct 02:30 CET",
	}
	for i, at := range got {
		if s := at.Format("Mon 2 Jan 15:04 MST"); s != want[i] {
			t.Errorf("firing %d is %s, and should be %s", i+1, s, want[i])
		}
	}

	// Said as the thing that actually goes wrong, not as a list of dates: two
	// firings on one day is two documents to one customer.
	onThe25th := 0
	for _, at := range got {
		if at.Day() == 25 && at.Month() == time.October {
			onThe25th++
		}
	}
	if onThe25th != 1 {
		t.Fatalf("fired %d times on the day the clocks go back — every recipient gets that many copies", onThe25th)
	}
}

/*
And the other half of the same rule: a schedule on a cadence is meant to fire in
both of them. A 25-hour day has 25 hours in it, and an hourly schedule that
skipped one would be this bug's mirror image — the fix for a duplicate quietly
becoming a gap.
*/
func TestAnHourlyScheduleFiresInBothPassesOfTheRepeatedHour(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no timezone database on this host")
	}

	p := planFor(t, "0 * * * *", "Europe/Berlin")
	// From midnight, far enough to cross the 03:00 CEST → 02:00 CET shift.
	got := firings(p, time.Date(2026, 10, 25, 0, 30, 0, 0, berlin), 4)

	want := []string{
		"25 Oct 01:00 CEST",
		"25 Oct 02:00 CEST",
		"25 Oct 02:00 CET", // the repeat, and it should fire
		"25 Oct 03:00 CET",
	}
	for i, at := range got {
		if s := at.Format("2 Jan 15:04 MST"); s != want[i] {
			t.Errorf("firing %d is %s, and should be %s", i+1, s, want[i])
		}
	}

	// The day is 25 hours long and the schedule says every hour.
	day := 0
	at := p.Next(time.Date(2026, 10, 24, 23, 59, 0, 0, berlin))
	for at.Day() == 25 && at.Month() == time.October {
		day++
		at = p.NextAfter(at, at.Add(time.Minute))
	}
	if day != 25 {
		t.Fatalf("an hourly schedule fired %d times in a 25-hour day", day)
	}
}

/*
The clocks going forward takes an hour away, and any firing inside it.

Left as it is, deliberately. The alternative to a report a day late is a report
that runs at a time nobody asked for, and the period tells the truth either way:
the run that follows covers 47 hours and is labelled with both dates, so the
statement says what it contains. Pinned so that it is a decision rather than
something nobody looked at.
*/
func TestWhenTheClocksGoForwardTheMissedHourRollsIntoTheNextRun(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no timezone database on this host")
	}

	p := planFor(t, "30 2 * * *", "Europe/Berlin")
	got := firings(p, time.Date(2026, 3, 27, 12, 0, 0, 0, berlin), 3)

	// 29 March has no 02:30 at all.
	for _, at := range got {
		if at.Day() == 29 && at.Month() == time.March {
			t.Fatalf("fired at %s, and that time does not exist", at.Format("2 Jan 15:04 MST"))
		}
	}

	// Nothing is lost: the run after covers the day that had no firing.
	start, end := p.Period(got[1])
	if span := end.Sub(start); span < 46*time.Hour || span > 48*time.Hour {
		t.Fatalf("the run after the missing day covers %v, and should cover the two days", span)
	}
	if label := Label(start, end); label != "28 Mar 2026 – 30 Mar 2026" {
		t.Fatalf("the period is labelled %q, which does not say it covers two days", label)
	}
}

/*
A monthly schedule crosses both shifts a year and must not notice either.

This is the demo's own cadence and the one most reports are on — the first of
the month at six — so the months either side of a shift being an hour long or
an hour short would show up on every invoice in Europe.
*/
func TestAMonthlyScheduleIsUnmovedByEitherShift(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no timezone database on this host")
	}

	p := planFor(t, "0 6 1 * *", "Europe/Berlin")
	for _, at := range firings(p, time.Date(2026, 1, 15, 0, 0, 0, 0, berlin), 12) {
		if h, m := at.Hour(), at.Minute(); h != 6 || m != 0 {
			t.Errorf("%s fires at %02d:%02d, and the schedule says six", at.Format("Jan"), h, m)
		}
		if at.Day() != 1 {
			t.Errorf("%s fires on the %d, and the schedule says the first", at.Format("Jan"), at.Day())
		}
		start, end := p.Period(at)
		// A month either side of a shift is 23 or 25 hours out; anything more
		// is the period drifting rather than the clocks moving.
		if span := end.Sub(start); span < 27*24*time.Hour || span > 32*24*time.Hour {
			t.Errorf("the period ending %s covers %v", at.Format("2 Jan"), span)
		}
		if got := Label(start, end); got != at.AddDate(0, 0, -1).Format("January 2006") {
			t.Errorf("the period ending %s is labelled %q", at.Format("2 Jan"), got)
		}
	}
}

/*
Period used to walk a year, one firing at a time, on every fire.

A monthly schedule is twelve steps and nobody noticed. A schedule running every
minute is half a million, which is a third of a second of CPU — once a minute,
per schedule, for an answer two steps away. It is the shape the pool bug had:
correct, and priced as though the cheap case were the only one.
*/
func TestPeriodDoesNotWalkAYearToFindTheLastFiring(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		what     string
		expr     string
		min, max time.Duration
	}{
		{"every minute", "* * * * *", time.Minute, time.Minute},
		{"hourly", "0 * * * *", time.Hour, time.Hour},
		{"daily", "0 6 * * *", 23 * time.Hour, 25 * time.Hour},
		{"monthly", "0 6 1 * *", 28 * 24 * time.Hour, 31 * 24 * time.Hour},
	} {
		t.Run(c.what, func(t *testing.T) {
			t.Parallel()
			p := planFor(t, c.expr, "UTC")
			at := p.Next(time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC))

			started := time.Now()
			start, end := p.Period(at)
			took := time.Since(started)

			// The answer first: fast and wrong is not the trade being made.
			if got := end.Sub(start); got < c.min || got > c.max {
				t.Fatalf("the period is %v, and the schedule runs %s", got, c.what)
			}
			if end != at.In(p.loc) {
				t.Fatalf("the period ends at %s and the firing was at %s", end, at)
			}
			// Generous — the point is a bound, not a benchmark, and a loaded
			// CI machine is slower than this one. The old code took 361ms.
			if took > 20*time.Millisecond {
				t.Fatalf("Period took %v for a schedule running %s, and it runs on every fire",
					took, c.what)
			}
		})
	}
}
