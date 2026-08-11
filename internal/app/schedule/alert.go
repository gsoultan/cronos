package schedule

import (
	"context"
	"time"
)

// Alerter tells a human that a run did not work.
//
// Declared here because the scheduler is what knows the outcome. A burst
// reports what it managed; only the loop above it knows that this was the
// monthly invoice run and that nobody is watching at six in the morning.
type Alerter interface {
	Alert(ctx context.Context, a Alert) error
}

// Alert is what somebody is told.
type Alert struct {
	To       string
	Schedule string
	Period   string
	// Recovered marks the alert that says it is working again. Sent because a
	// pager that only ever fires is one people mute — the all-clear is what
	// makes the next alarm worth reading.
	Recovered bool

	Recipients int
	Delivered  int
	Failures   []string
	Err        string
}

// AlertInterval is how often the same failing schedule may alert.
//
// A schedule that fails every minute produces an alert every minute, and a
// mailbox with four hundred identical alerts in it is a mailbox where the
// four hundred and first is not read. An hour is long enough to be quiet and
// short enough that a morning's failure is not discovered in the afternoon.
const AlertInterval = time.Hour

// alerts decides whether to send, and remembers what it already said.
type alerts struct {
	sent map[string]time.Time
	bad  map[string]bool
}

func newAlerts() *alerts {
	return &alerts{sent: map[string]time.Time{}, bad: map[string]bool{}}
}

// should reports whether this outcome is worth telling somebody about.
//
// Every change of state is, immediately: the first failure and the recovery
// after it. A continuing failure is repeated at most once an interval, because
// the second identical alert carries no information the first did not.
func (a *alerts) should(schedule string, failing bool, now time.Time) (send, recovered bool) {
	was := a.bad[schedule]
	a.bad[schedule] = failing

	switch {
	case failing && !was:
		a.sent[schedule] = now
		return true, false
	case !failing && was:
		delete(a.sent, schedule)
		return true, true
	case !failing:
		return false, false
	}

	if last, ok := a.sent[schedule]; !ok || now.Sub(last) >= AlertInterval {
		a.sent[schedule] = now
		return true, false
	}
	return false, false
}
