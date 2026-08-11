package email

import (
	"context"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/gsoultan/cronos/internal/app/schedule"
)

type captured struct{ d burst.Delivery }

func (c *captured) Deliver(_ context.Context, d burst.Delivery) error {
	c.d = d
	return nil
}

func send(t *testing.T, a schedule.Alert) burst.Delivery {
	t.Helper()
	c := &captured{}
	if err := New(c).Alert(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	return c.d
}

// Whoever reads this is reading a notification list on a phone, so the useful
// half has to survive being cut off.
func TestTheSubjectLeadsWithWhatHappened(t *testing.T) {
	failed := send(t, schedule.Alert{To: "ops@acme.example", Schedule: "monthly",
		Period: "July 2026", Err: "no such dataset"})
	if !strings.HasPrefix(failed.Subject, "FAILED: monthly") {
		t.Errorf("subject = %q", failed.Subject)
	}

	partial := send(t, schedule.Alert{To: "ops@acme.example", Schedule: "monthly",
		Period: "July 2026", Recipients: 5000, Delivered: 4997,
		Failures: []string{"a", "b", "c"}})
	if !strings.HasPrefix(partial.Subject, "INCOMPLETE: monthly delivered 4997 of 5000") {
		t.Errorf("subject = %q", partial.Subject)
	}

	ok := send(t, schedule.Alert{To: "ops@acme.example", Schedule: "monthly",
		Period: "July 2026", Recovered: true, Recipients: 5000})
	if !strings.HasPrefix(ok.Subject, "Recovered: monthly") {
		t.Errorf("subject = %q", ok.Subject)
	}
}

// An alert holding five thousand identical lines is one nobody scrolls to the
// end of, and the run record has them all anyway.
func TestTheFailureListIsCapped(t *testing.T) {
	failures := make([]string, 300)
	for i := range failures {
		failures[i] = "recipient failed"
	}
	d := send(t, schedule.Alert{To: "ops@acme.example", Schedule: "monthly",
		Recipients: 300, Failures: failures})

	if strings.Count(d.Body, "recipient failed") > 25 {
		t.Errorf("listed %d failures", strings.Count(d.Body, "recipient failed"))
	}
	if !strings.Contains(d.Body, "and 280 more") {
		t.Errorf("it did not say how many were omitted:\n%s", d.Body)
	}
}

// The point of the message: what to do about it.
func TestTheBodySaysWhatWasAndWasNotDelivered(t *testing.T) {
	d := send(t, schedule.Alert{To: "ops@acme.example", Schedule: "monthly",
		Period: "July 2026", Recipients: 10, Delivered: 8, Failures: []string{"x", "y"}})

	for _, want := range []string{"delivered 8 of 10", "2 recipients did not receive",
		"are not resent"} {
		if !strings.Contains(d.Body, want) {
			t.Errorf("body does not say %q:\n%s", want, d.Body)
		}
	}
}

// An alert is a delivery with no attachment, so nothing tries to attach a
// document that does not exist.
func TestAnAlertCarriesNoDocument(t *testing.T) {
	d := send(t, schedule.Alert{To: "ops@acme.example", Schedule: "monthly", Err: "boom"})
	if len(d.Document) != 0 || d.Filename != "" {
		t.Errorf("delivery = %+v", d)
	}
}
