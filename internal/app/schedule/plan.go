package schedule

import (
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/robfig/cron/v3"
)

// Plan is one schedule, parsed and ready to be asked when it next runs.
type Plan struct {
	Schedule definition.Schedule
	spec     cron.Schedule
	loc      *time.Location
	// cadence is the schedule's own rhythm — the gap between two firings, and
	// nothing to do with any particular date. Measured once, at parse.
	cadence time.Duration
}

// Parse resolves a schedule's cron expression and timezone.
//
// Both at load, not at fire: a timezone the host does not have or a cron
// expression that will not parse should stop a deployment, not surprise
// somebody at six in the morning on the first of the month.
func Parse(s definition.Schedule) (Plan, error) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return Plan{}, fmt.Errorf("%w: schedule %q wants %q: %v",
			ErrBadTimezone, s.Name, s.Timezone, err)
	}
	spec, err := cron.ParseStandard(s.Cron)
	if err != nil {
		return Plan{}, fmt.Errorf("%w: schedule %q has %q: %v", ErrBadCron, s.Name, s.Cron, err)
	}
	return Plan{Schedule: s, spec: spec, loc: loc, cadence: cadenceOf(spec)}, nil
}

// cadenceOf is the gap between two consecutive firings.
//
// Measured in UTC, where no clock has ever moved, so the answer is the
// schedule's own rhythm rather than one contaminated by whatever shift is
// being reasoned about. From a fixed instant, so it is the same number on
// every machine on every day.
//
// Zero for an expression that never fires again — "the 30th of February" parses
// and is a schedule with no next occurrence. Callers treat zero as "no idea".
func cadenceOf(spec cron.Schedule) time.Duration {
	from := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	first := spec.Next(from)
	if first.IsZero() {
		return 0
	}
	second := spec.Next(first)
	if second.IsZero() {
		return 0
	}
	return second.Sub(first)
}

// Next is when this schedule fires after t.
//
// Computed in the schedule's own location. "The first of the month at six" is
// a local claim: run it in UTC and a Berlin customer gets a statement dated an
// hour into the previous month twice a year.
func (p Plan) Next(t time.Time) time.Time {
	return p.spec.Next(t.In(p.loc))
}

/*
NextAfter is when this schedule fires again, having just fired at `fired`.

Not Next(now), which is what it was, and which sends everybody two.

When a zone puts its clocks back, an hour of wall-clock time happens twice. A
schedule that names a time inside it — "half past two", which in Berlin is the
25th of October — comes round a second time an hour later, and Next returns it.
The first firing sent eight hundred customers their statement; an hour later
the same run fired again and sent them all a second copy, for a period an hour
long. Once a year, in the zones that shift, for every schedule whose time falls
in the repeated hour.

Vixie cron has the same rule and has had it for decades: when the clock goes
back, a job at a fixed time runs once. A job on a cadence — every minute, every
hour — runs at every occurrence, because a 25-hour day genuinely has 25 hours in
it and skipping one would be the other bug.

Which one this is comes from the cadence rather than from reading the
expression. The repeat arrives exactly one shift later, so if the schedule's own
rhythm is longer than that gap, the second occurrence is the same firing wearing
a different offset. An hourly schedule's is not, and it fires.

The spring forward is left alone. An hour that does not happen takes any firing
inside it with it, and the next run's period stretches to cover the gap and is
labelled with it — a report a day late once a year, correctly described, rather
than a duplicate.
*/
func (p Plan) NextAfter(fired, now time.Time) time.Time {
	next := p.Next(now)
	if p.cadence <= 0 || !sameWallClock(next.In(p.loc), fired.In(p.loc)) {
		return next
	}
	if next.Sub(fired) >= p.cadence {
		// It fires this often anyway. The clock moving is not why this one is
		// due — an hourly schedule at 02:00 is meant to run in both of them.
		return next
	}
	return p.Next(next)
}

// sameWallClock is whether two instants read the same on a clock on the wall,
// which across a shift is two different instants.
func sameWallClock(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd &&
		a.Hour() == b.Hour() && a.Minute() == b.Minute()
}

// Period is the span a firing at t covers: since the previous firing.
//
// Derived from the cadence rather than declared, because a schedule already
// says how often it runs and saying it twice is two things to keep in step.
// It also means an author who changes "monthly" to "weekly" does not have to
// remember to change a period expression too.
func (p Plan) Period(t time.Time) (start, end time.Time) {
	end = t.In(p.loc)

	// Start a couple of firings back and widen if that was not enough, rather
	// than a year back whatever the cadence. It used to be a year always, and
	// the walk forward is one Next per firing in between: a schedule running
	// every minute stepped through half a million of them to find the one it
	// had just passed — a third of a second of CPU, once a minute, per
	// schedule, for an answer two steps away.
	back := 2 * p.cadence
	if back <= 0 {
		back = 365 * 24 * time.Hour // no cadence to go on; the old probe
	}
	for {
		probe := end.Add(-back)
		next := p.spec.Next(probe)
		if next.Before(end) {
			// Something fires in between. Walk to the last one before end,
			// which with a probe this close is a step or two.
			for {
				later := p.spec.Next(next)
				if !later.Before(end) {
					return next.In(p.loc), end
				}
				next = later
			}
		}
		// Nothing fires between the probe and here. Widen, up to the year the
		// probe used to start at — past which the honest answer is the probe
		// itself, which is what it always was.
		if back >= 365*24*time.Hour {
			return probe.In(p.loc), end
		}
		back *= 2
	}
}

// Label names the period the way a person would: "July 2026" for a month,
// a date range otherwise.
//
// It appears in a subject line and a filename, so it is written for the
// recipient rather than for a log.
func Label(start, end time.Time) string {
	span := end.Sub(start)
	switch {
	case span > 27*24*time.Hour && span < 32*24*time.Hour:
		// A month, whichever month it actually was. The period ends at the
		// firing, so the month being reported is the one that just closed.
		return start.Format("January 2006")
	case span > 6*24*time.Hour && span < 8*24*time.Hour:
		return fmt.Sprintf("%s – %s", start.Format("2 Jan"), end.AddDate(0, 0, -1).Format("2 Jan 2006"))
	case span >= 23*time.Hour && span <= 25*time.Hour:
		return start.Format("2 January 2006")
	}
	return fmt.Sprintf("%s – %s", start.Format("2 Jan 2006"), end.Format("2 Jan 2006"))
}
