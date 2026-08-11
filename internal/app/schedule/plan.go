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
	return Plan{Schedule: s, spec: spec, loc: loc}, nil
}

// Next is when this schedule fires after t.
//
// Computed in the schedule's own location. "The first of the month at six" is
// a local claim: run it in UTC and a Berlin customer gets a statement dated an
// hour into the previous month twice a year.
func (p Plan) Next(t time.Time) time.Time {
	return p.spec.Next(t.In(p.loc))
}

// Period is the span a firing at t covers: since the previous firing.
//
// Derived from the cadence rather than declared, because a schedule already
// says how often it runs and saying it twice is two things to keep in step.
// It also means an author who changes "monthly" to "weekly" does not have to
// remember to change a period expression too.
func (p Plan) Period(t time.Time) (start, end time.Time) {
	end = t.In(p.loc)
	// Walk back far enough that Next lands on the firing before this one. A
	// day short of the widest cadence anyone schedules, then step forward.
	probe := end.AddDate(-1, 0, 0)
	for {
		next := p.spec.Next(probe)
		if !next.Before(end) {
			return probe.In(p.loc), end
		}
		probe = next
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
