package schedule_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/gsoultan/cronos/internal/app/schedule"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

func monthly() definition.Schedule {
	return definition.Schedule{
		Name: "monthly-statements", Report: "statement", Output: "pdf",
		Cron: "0 6 1 * *", Timezone: "Europe/Berlin",
		Deliver: []definition.DeliverSpec{{Via: "file", To: "x"}},
	}
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// -- Parsing ---------------------------------------------------------------

// "The first of the month at six" is a local claim. Run it in UTC and a Berlin
// customer gets a statement dated an hour into the previous month twice a year.
func TestNextIsComputedInTheSchedulesOwnTimezone(t *testing.T) {
	p, err := schedule.Parse(monthly())
	if err != nil {
		t.Fatal(err)
	}

	// Mid-July UTC. Berlin is +02:00 in summer, so 06:00 local is 04:00Z.
	next := p.Next(at("2026-07-15T12:00:00Z"))
	if got := next.Format(time.RFC3339); got != "2026-08-01T06:00:00+02:00" {
		t.Errorf("next = %s, want 06:00 Berlin on the 1st", got)
	}
}

func TestParseRefuses(t *testing.T) {
	bad := monthly()
	bad.Timezone = "Middle/Earth"
	if _, err := schedule.Parse(bad); !errors.Is(err, schedule.ErrBadTimezone) {
		t.Errorf("got %v, want ErrBadTimezone", err)
	}

	bad = monthly()
	bad.Cron = "every month please"
	if _, err := schedule.Parse(bad); !errors.Is(err, schedule.ErrBadCron) {
		t.Errorf("got %v, want ErrBadCron", err)
	}
}

// A schedule says when it runs, not what period it covers. Saying it twice is
// two things to keep in step.
func TestThePeriodIsTheSpanSinceTheLastFiring(t *testing.T) {
	p, _ := schedule.Parse(monthly())

	start, end := p.Period(at("2026-08-01T04:00:00Z"))
	if got := start.Format(time.DateOnly); got != "2026-07-01" {
		t.Errorf("period starts %s, want the previous firing", got)
	}
	if got := end.Format(time.DateOnly); got != "2026-08-01" {
		t.Errorf("period ends %s, want this firing", got)
	}
	// It appears in a subject line, so it is written for the recipient.
	if got := schedule.Label(start, end); got != "July 2026" {
		t.Errorf("label = %q, want July 2026", got)
	}
}

func TestLabelsReadLikeAPersonWroteThem(t *testing.T) {
	daily, _ := schedule.Parse(definition.Schedule{
		Name: "d", Report: "r", Output: "o", Cron: "0 6 * * *", Timezone: "UTC",
		Deliver: []definition.DeliverSpec{{Via: "file", To: "x"}}})

	start, end := daily.Period(at("2026-07-15T06:00:00Z"))
	if got := schedule.Label(start, end); got != "14 July 2026" {
		t.Errorf("a daily label = %q", got)
	}
}

// -- The loop --------------------------------------------------------------

type source []definition.Schedule

func (s source) Schedules() []definition.Schedule { return s }

type owner struct{}

func (owner) Owner(definition.Schedule) principal.Principal {
	return principal.Principal{Subject: "scheduler", ProjectRole: principal.ProjectEditor}
}

// runner records what it was asked to do, and can be made slow or angry.
type runner struct {
	mu     sync.Mutex
	runs   []burst.Run
	block  chan struct{}
	result burst.Result
	err    error
}

func (r *runner) Run(_ context.Context, _ definition.Schedule, run burst.Run,
	_ principal.Principal) (burst.Result, error) {

	r.mu.Lock()
	r.runs = append(r.runs, run)
	block := r.block
	r.mu.Unlock()

	if block != nil {
		<-block
	}
	return r.result, r.err
}

func (r *runner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

func (r *runner) last() burst.Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.runs) == 0 {
		return nil
	}
	return r.runs[len(r.runs)-1]
}

func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// clock is a hand-wound time source, so the loop can be driven through months
// without waiting for one.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

func start(t *testing.T, r *runner, c *clock, scheds ...definition.Schedule) *schedule.Service {
	t.Helper()
	svc := schedule.New(source(scheds), r, owner{}, quiet()).
		WithClock(c.now).WithTick(5 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = svc.Start(ctx) }()
	t.Cleanup(func() { cancel(); <-done })

	// Wait until it has armed. Moving the clock before the first arm() would
	// have it compute the next firing from the *new* time, so the one the test
	// is waiting for is already in the past and never comes.
	armed(t, svc, 1)
	return svc
}

// armed blocks until n schedules have a next firing.
func armed(t *testing.T, svc *schedule.Service, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(svc.Due()) >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("only %d schedules armed, want %d", len(svc.Due()), n)
}

func eventually(t *testing.T, want int, get func() int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if get() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("got %d, want %d", get(), want)
}

func TestADueScheduleFires(t *testing.T) {
	c := &clock{t: at("2026-07-15T12:00:00Z")}
	r := &runner{result: burst.Result{Recipients: 3, Delivered: 3}}
	start(t, r, c, monthly())

	// Nothing yet: the next firing is the first of August.
	time.Sleep(50 * time.Millisecond)
	if r.count() != 0 {
		t.Fatalf("fired %d times before it was due", r.count())
	}

	c.set(at("2026-08-01T05:00:00Z")) // past 06:00 Berlin
	eventually(t, 1, r.count)

	run := r.last()
	if run["periodLabel"] != "July 2026" {
		t.Errorf("period = %q, want July 2026", run["periodLabel"])
	}
	if run["periodStart"] != "2026-07-01" || run["periodEnd"] != "2026-08-01" {
		t.Errorf("period = %s .. %s", run["periodStart"], run["periodEnd"])
	}
}

// A server down for a week comes back and runs each schedule once. Seven
// copies of last week's invoices is not a recovery.
func TestItDoesNotCatchUpOnMissedFirings(t *testing.T) {
	c := &clock{t: at("2026-07-15T12:00:00Z")}
	r := &runner{}
	start(t, r, c, monthly())

	// Three months pass while nothing was watching.
	c.set(at("2026-10-15T12:00:00Z"))
	eventually(t, 1, r.count)

	time.Sleep(80 * time.Millisecond)
	if got := r.count(); got != 1 {
		t.Errorf("fired %d times for three missed months, want 1", got)
	}
}

// Two bursts of the same statements racing each other deliver every customer
// two documents that disagree.
func TestARunStillGoingIsNotOverlapped(t *testing.T) {
	c := &clock{t: at("2026-07-31T12:00:00Z")}
	r := &runner{block: make(chan struct{})}
	start(t, r, c, monthly())

	c.set(at("2026-08-01T05:00:00Z"))
	eventually(t, 1, r.count)

	// Another month goes by while the first run is still in flight.
	c.set(at("2026-09-01T05:00:00Z"))
	time.Sleep(80 * time.Millisecond)
	if got := r.count(); got != 1 {
		t.Errorf("started %d runs while one was going", got)
	}

	// Letting it finish does not retroactively run the firing that was skipped
	// — that occurrence is gone, which is the policy. What must happen is that
	// the schedule recovers and fires at its next one.
	close(r.block)
	time.Sleep(80 * time.Millisecond)
	if got := r.count(); got != 1 {
		t.Errorf("the skipped firing was run after the fact (%d runs)", got)
	}

	c.set(at("2026-10-01T05:00:00Z"))
	eventually(t, 2, r.count)
}

// Publishing a schedule arms it. Needing a restart for that would be strange
// when the definitions themselves reload.
func TestSchedulesPublishedLaterAreArmed(t *testing.T) {
	c := &clock{t: at("2026-07-15T12:00:00Z")}
	r := &runner{}

	live := &mutableSource{}
	svc := schedule.New(live, r, owner{}, quiet()).
		WithClock(c.now).WithTick(5 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = svc.Start(ctx) }()
	defer func() { cancel(); <-done }()

	live.set(monthly())
	armed(t, svc, 1)

	c.set(at("2026-08-01T05:00:00Z"))
	eventually(t, 1, r.count)
}

type mutableSource struct {
	mu   sync.Mutex
	list []definition.Schedule
}

func (m *mutableSource) Schedules() []definition.Schedule {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]definition.Schedule(nil), m.list...)
}

func (m *mutableSource) set(s ...definition.Schedule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.list = s
}

// One unparseable schedule must not stop the other nine from running.
func TestABrokenScheduleDoesNotStopTheOthers(t *testing.T) {
	broken := monthly()
	broken.Name = "broken"
	broken.Timezone = "Middle/Earth"

	c := &clock{t: at("2026-07-15T12:00:00Z")}
	r := &runner{}
	svc := start(t, r, c, broken, monthly())

	if got := len(svc.Due()); got != 1 {
		t.Errorf("%d schedules armed, want only the working one", got)
	}
	c.set(at("2026-08-01T05:00:00Z"))
	eventually(t, 1, r.count)
}

// A startup should fail loudly rather than serve with two of its five
// schedules quietly missing.
func TestCheckReportsASchedulesThatWillNotArm(t *testing.T) {
	broken := monthly()
	broken.Cron = "nope"

	if err := schedule.Check(source{monthly()}); err != nil {
		t.Errorf("a working schedule should check out: %v", err)
	}
	if err := schedule.Check(source{monthly(), broken}); err == nil {
		t.Error("a schedule that will not parse was reported as fine")
	}
}

func TestDueReportsWhatAnOperatorAsksFor(t *testing.T) {
	c := &clock{t: at("2026-07-15T12:00:00Z")}
	svc := start(t, &runner{}, c, monthly())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if when, ok := svc.Due()["monthly-statements"]; ok {
			if got := when.Format(time.RFC3339); got != "2026-08-01T06:00:00+02:00" {
				t.Errorf("due at %s", got)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("nothing was armed")
}
