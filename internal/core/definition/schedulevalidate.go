package definition

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// cronFields is the shape a schedule's cron expression must have.
//
// Five fields, standard order. Validated here rather than trusted to the
// scheduler, because "0 6 1 * *" and "0 6 * * 1" differ by one character and
// mean monthly versus weekly — a mistake that shows up as five times the
// invoices, on the wrong days, in production.
var cronFields = regexp.MustCompile(`^\S+(\s+\S+){4}$`)

// Validate reports every reason the schedule cannot be stored.
func (s Schedule) Validate() error {
	switch {
	case !slug.MatchString(s.Name):
		return fmt.Errorf("%w: name %q must be lowercase letters, digits and dashes", ErrInvalid, s.Name)
	case !slug.MatchString(s.Report):
		return fmt.Errorf("%w: schedule %q names no report", ErrInvalid, s.Name)
	case s.Output == "":
		// Without one, the schedule would pick an output for the author, and a
		// statement sent as a spreadsheet because that happened to be first is
		// a mistake nobody catches until a customer opens it.
		return fmt.Errorf("%w: schedule %q names no output", ErrInvalid, s.Name)
	case !cronFields.MatchString(strings.TrimSpace(s.Cron)):
		return fmt.Errorf("%w: schedule %q has cron %q, want five fields", ErrInvalid, s.Name, s.Cron)
	case s.Timezone == "":
		return fmt.Errorf("%w: schedule %q has no timezone — \"the first of the month\" is a local claim",
			ErrInvalid, s.Name)
	case len(s.Deliver) == 0:
		// A schedule that renders and delivers nowhere is a cron job that
		// heats the datacentre.
		return fmt.Errorf("%w: schedule %q delivers nowhere", ErrInvalid, s.Name)
	}
	/*
	   And a timezone this build can actually resolve.

	   Checked here rather than left to the scheduler, because of what happens
	   when it is not: the name is only checked for emptiness, so
	   "Europe/Berln" publishes with a 200 and the running instance carries on
	   perfectly well. Then the next restart — a deploy, an eviction, an OOM —
	   finds a schedule that will not arm and refuses to start at all. The
	   deployment is down, the API with it, and the only way to remove the
	   typo is a psql prompt.

	   So an editor could take the whole deployment out with a misspelled
	   timezone, and nothing would connect the outage to a definition somebody
	   published three weeks earlier. Rejecting it at the moment it is typed
	   costs one function call and is the only place the person who made the
	   mistake is still holding it.
	*/
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("%w: schedule %q wants timezone %q, which this build does not know",
			ErrInvalid, s.Name, s.Timezone)
	}

	if err := s.validateBurst(); err != nil {
		return err
	}
	return s.validateDeliveries()
}

func (s Schedule) validateBurst() error {
	if s.Burst == nil {
		return nil
	}
	if !slug.MatchString(s.Burst.Over.Dataset) {
		return fmt.Errorf("%w: schedule %q bursts over %q, which is not a dataset name",
			ErrInvalid, s.Name, s.Burst.Over.Dataset)
	}
	if len(s.Burst.Bind) == 0 {
		// Without a binding every recipient gets the same document, which is
		// a burst producing N copies of one report rather than N reports.
		return fmt.Errorf("%w: schedule %q bursts but binds nothing from the row", ErrInvalid, s.Name)
	}
	for param := range s.Burst.Bind {
		if !identifier.MatchString(param) {
			return fmt.Errorf("%w: schedule %q binds %q, which is not a parameter name",
				ErrInvalid, s.Name, param)
		}
	}
	if s.Burst.Concurrency < 0 {
		return fmt.Errorf("%w: schedule %q has negative concurrency", ErrInvalid, s.Name)
	}
	return nil
}

func (s Schedule) validateDeliveries() error {
	for i, d := range s.Deliver {
		if d.Via == "" {
			return fmt.Errorf("%w: schedule %q delivery %d names no channel", ErrInvalid, s.Name, i)
		}
		if d.To == "" {
			// A channel with no destination delivers to whatever its default
			// is, and a default recipient for someone else's invoices is the
			// worst possible one.
			return fmt.Errorf("%w: schedule %q delivery %d has no destination", ErrInvalid, s.Name, i)
		}
	}
	return nil
}
