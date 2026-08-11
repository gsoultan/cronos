// Package email sends schedule alerts to a person.
package email

import (
	"context"
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/gsoultan/cronos/internal/app/schedule"
)

// Sender is the one thing an alert needs: somewhere to put a message.
//
// The delivery channel already is one, so an alert is a delivery with no
// attachment rather than a second SMTP client to configure and keep patched.
type Sender interface {
	Deliver(ctx context.Context, d burst.Delivery) error
}

// Alerter emails whoever a schedule's onFailure names.
type Alerter struct {
	send Sender
}

// New returns an Alerter over a delivery channel.
func New(s Sender) *Alerter { return &Alerter{send: s} }

// Alert sends one.
func (a *Alerter) Alert(ctx context.Context, al schedule.Alert) error {
	return a.send.Deliver(ctx, burst.Delivery{
		To: al.To, Subject: subject(al), Body: body(al),
	})
}

// subject says what happened and which schedule, in that order.
//
// Whoever reads this is reading a notification list on a phone, so the useful
// half has to survive being cut off after forty characters.
func subject(a schedule.Alert) string {
	if a.Recovered {
		return fmt.Sprintf("Recovered: %s delivered %s", a.Schedule, a.Period)
	}
	if a.Err != "" {
		return fmt.Sprintf("FAILED: %s did not run for %s", a.Schedule, a.Period)
	}
	return fmt.Sprintf("INCOMPLETE: %s delivered %d of %d for %s",
		a.Schedule, a.Delivered, a.Recipients, a.Period)
}

// body says what to do about it.
//
// The failures are listed rather than counted, and capped: an alert holding
// five thousand identical error lines is one nobody scrolls to the end of, and
// the run record has them all anyway.
func body(a schedule.Alert) string {
	var b strings.Builder

	if a.Recovered {
		fmt.Fprintf(&b, "%s delivered to all %d recipients for %s.\n\n",
			a.Schedule, a.Recipients, a.Period)
		b.WriteString("This is the all-clear for the failures reported earlier.\n")
		return b.String()
	}

	if a.Err != "" {
		fmt.Fprintf(&b, "%s did not run for %s.\n\n  %s\n\n", a.Schedule, a.Period, a.Err)
		b.WriteString("Nothing was delivered. The run is recorded under /v1/runs.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "%s delivered %d of %d documents for %s.\n\n",
		a.Schedule, a.Delivered, a.Recipients, a.Period)
	fmt.Fprintf(&b, "%d recipients did not receive theirs:\n\n", len(a.Failures))

	const most = 20
	for i, f := range a.Failures {
		if i == most {
			fmt.Fprintf(&b, "  … and %d more — see /v1/runs for every one.\n", len(a.Failures)-most)
			break
		}
		fmt.Fprintf(&b, "  %s\n", f)
	}
	b.WriteString("\nThe successful documents were delivered and are not resent.\n")
	return b.String()
}
