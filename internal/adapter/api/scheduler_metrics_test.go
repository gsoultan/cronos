package api_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
)

/*
A scheduler that has stopped is visible.

Every alert this product documented counted things that happened — runs,
deliveries, failures. A scheduler that is not running produces none of them, and
zero failures is what a perfect night looks like as well as what a dead loop
looks like. No alert written against a counter can tell those apart, so the
failure mode was: the one instance with CRONOS_SCHEDULER=1 stops scheduling, and
the first person to know is a customer who did not get an invoice.

The process is still serving. Health is 200, readiness is ok, every request
works. That is what makes this the hardest thing here to see and the most
expensive to miss.
*/

// stuck is a scheduler that armed some schedules and then stopped going round.
type stuck struct {
	due    map[string]time.Time
	last   time.Time
	demote bool
}

func (s stuck) Due() map[string]time.Time { return s.due }
func (s stuck) LastTick() time.Time       { return s.last }
func (s stuck) Leading() bool             { return !s.demote }

func scrape(t *testing.T, m *api.Metrics) string {
	t.Helper()
	w := httptest.NewRecorder()
	m.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/metrics", nil))
	return w.Body.String()
}

// gauge reads one labelled value out of the exposition.
func gauge(t *testing.T, body, name, project string) float64 {
	t.Helper()
	re := regexp.MustCompile(regexp.QuoteMeta(name) + `\{project="` +
		regexp.QuoteMeta(project) + `"\} ([0-9.e+-]+)`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("%s{project=%q} is not in the exposition:\n%s", name, project, body)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAStoppedSchedulerShowsUp(t *testing.T) {
	now := time.Date(2026, 8, 1, 6, 30, 0, 0, time.UTC)
	m := api.NewMetrics().WithClock(func() time.Time { return now })

	// Armed an hour ago and not seen since. Everything else about this process
	// is healthy.
	m.WatchScheduler("acme/finance", stuck{
		due:  map[string]time.Time{"monthly-statements": now.Add(-25 * time.Minute)},
		last: now.Add(-time.Hour),
	})

	body := scrape(t, m)

	if got := gauge(t, body, "cronos_scheduler_seconds_since_tick", "acme/finance"); got < 3500 {
		t.Errorf("seconds since the last pass = %.0f, want about an hour", got)
	}
	// And the business-level consequence, which is the number somebody pages on:
	// a statement run that should have gone twenty-five minutes ago.
	if got := gauge(t, body, "cronos_schedule_overdue_seconds", "acme/finance"); got < 1400 {
		t.Errorf("most overdue = %.0f seconds, want about twenty-five minutes", got)
	}
}

// A scheduler going round normally reads as fine, or the alert is noise and
// somebody turns it off.
func TestAWorkingSchedulerLooksFine(t *testing.T) {
	now := time.Date(2026, 8, 1, 6, 30, 0, 0, time.UTC)
	m := api.NewMetrics().WithClock(func() time.Time { return now })

	m.WatchScheduler("acme/finance", stuck{
		// Next firing is in the future, and the loop passed five seconds ago.
		due:  map[string]time.Time{"monthly-statements": now.Add(24 * time.Hour)},
		last: now.Add(-5 * time.Second),
	})

	body := scrape(t, m)
	if got := gauge(t, body, "cronos_scheduler_seconds_since_tick", "acme/finance"); got > 30 {
		t.Errorf("a healthy loop reported %.0f seconds since its last pass", got)
	}
	if got := gauge(t, body, "cronos_schedule_overdue_seconds", "acme/finance"); got != 0 {
		t.Errorf("nothing is due and overdue = %.0f", got)
	}
}

/*
And a deployment where nobody armed anything at all.

The other way for the answer to be no, and the one an operator hits on their
first install: CRONOS_SCHEDULER defaults to off, so a deployment that never set
it runs no schedules on any replica. Every counter stays at zero, which is
indistinguishable from a quiet week.

`cronos_scheduler_armed` is 0 on every instance in that case, which is the one
thing that can be alerted on across a fleet: if no instance reports 1, nobody
is scheduling.
*/
func TestAnUnarmedProcessSaysSo(t *testing.T) {
	m := api.NewMetrics()
	body := scrape(t, m)

	if !regexp.MustCompile(`cronos_scheduler_armed 0`).MatchString(body) {
		t.Fatalf("an unarmed process does not say so:\n%s", body)
	}
	// And it does not invent per-project series for schedulers it does not run.
	if regexp.MustCompile(`cronos_schedules_armed\{`).MatchString(body) {
		t.Error("a process with no scheduler reported per-project schedule gauges")
	}
}

func TestAnArmedProcessSaysSo(t *testing.T) {
	m := api.NewMetrics()
	m.WatchScheduler("acme/finance", stuck{due: map[string]time.Time{"a": {}, "b": {}}})

	body := scrape(t, m)
	if !regexp.MustCompile(`cronos_scheduler_armed 1`).MatchString(body) {
		t.Fatalf("an armed process does not say so:\n%s", body)
	}
	if got := gauge(t, body, "cronos_schedules_armed", "acme/finance"); got != 2 {
		t.Errorf("armed schedules = %.0f, want 2", got)
	}
}

/*
A scheduler that has never ticked is not reported as fifty-six years behind.

The zero time is a real state — the loop is starting, or it failed before its
first pass — and reporting it as seconds since 1970 is true and makes every
dashboard unreadable. Uptime is the honest bound: it cannot have ticked before
the process existed.
*/
func TestASchedulerThatNeverRanIsBoundedByUptime(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	now := start
	m := api.NewMetrics().WithClock(func() time.Time { return now })
	m.WatchScheduler("acme/finance", stuck{})

	now = start.Add(90 * time.Second)
	if got := gauge(t, scrape(t, m), "cronos_scheduler_seconds_since_tick", "acme/finance"); got != 90 {
		t.Fatalf("never-ticked reported %.0f seconds, want the uptime (90)", got)
	}
}
