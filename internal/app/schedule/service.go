package schedule

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Runner is what a due schedule fires.
type Runner interface {
	Run(ctx context.Context, s definition.Schedule, run burst.Run,
		pr principal.Principal) (burst.Result, error)
}

// Source supplies the schedules to arm.
//
// Read on every tick rather than once at start, so publishing a schedule arms
// it without a restart — the same property the definition repository already
// has, and it would be strange for schedules alone to need a deployment.
type Source interface {
	Schedules() []definition.Schedule
}

// Owner resolves the principal a schedule runs as.
//
// A schedule runs as somebody: the project member who owns it. It is not a
// superuser and it is not scope-less by accident — see docs/tenancy.md.
type Owner interface {
	Owner(s definition.Schedule) principal.Principal
}

// Service arms schedules and fires them when due.
type Service struct {
	source  Source
	runner  Runner
	owner   Owner
	log     *slog.Logger
	now     func() time.Time
	tick    time.Duration
	mu      sync.Mutex
	running map[string]bool
	due     map[string]time.Time
}

// Tick is how often the loop looks for work.
//
// A minute, because cron's resolution is a minute. Anything finer is a busy
// loop asking the same question; anything coarser can miss a firing.
const Tick = time.Minute

// New wires a Service.
func New(src Source, r Runner, o Owner, log *slog.Logger) *Service {
	return &Service{
		source: src, runner: r, owner: o, log: log,
		now: time.Now, tick: Tick,
		running: map[string]bool{},
		due:     map[string]time.Time{},
	}
}

// WithClock and WithTick make the loop testable without waiting for a minute
// of real time to pass.
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }
func (s *Service) WithTick(d time.Duration) *Service       { s.tick = d; return s }

// Start runs until the context is cancelled.
//
// In-flight bursts finish. Killing one mid-render leaves a delivery that half
// happened, which is worse to reconcile than one that did not start.
func (s *Service) Start(ctx context.Context) error {
	s.arm(s.now())
	s.log.Info("scheduler started", "schedules", len(s.due), "tick", s.tick)

	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("scheduler draining")
			return nil
		case <-ticker.C:
			s.fireDue(ctx, &wg)
		}
	}
}

// arm computes the first due time for every schedule.
//
// From now forward, never backward. A server that was down for a week comes
// back and runs each schedule once, at its next due time — seven copies of
// last week's invoices is not a recovery, it is an incident.
func (s *Service) arm(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fresh := map[string]time.Time{}
	for _, sched := range s.source.Schedules() {
		plan, err := Parse(sched)
		if err != nil {
			// Logged and skipped rather than fatal: one unparseable schedule
			// must not stop the other nine from running.
			s.log.Error("schedule not armed", "schedule", sched.Name, "err", err)
			continue
		}
		if when, ok := s.due[sched.Name]; ok {
			fresh[sched.Name] = when // keep what an armed schedule was waiting for
			continue
		}
		fresh[sched.Name] = plan.Next(now)
		s.log.Info("armed", "schedule", sched.Name, "next", fresh[sched.Name].Format(time.RFC3339))
	}
	s.due = fresh
}

// fireDue runs everything whose time has come.
func (s *Service) fireDue(ctx context.Context, wg *sync.WaitGroup) {
	now := s.now()
	s.arm(now) // pick up anything published since the last tick

	for _, sched := range s.source.Schedules() {
		plan, err := Parse(sched)
		if err != nil {
			continue
		}
		when, ok := s.claim(sched.Name, now)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(p Plan, at time.Time) {
			defer wg.Done()
			// Re-armed from now, not from the time that just fired. Stepping
			// forward one occurrence at a time would work through every missed
			// month in a loop — three months of downtime became three bursts,
			// which is the catch-up this is supposed not to do. It also means a
			// run that overran its own interval does not immediately requeue.
			defer func() { s.release(p.Schedule.Name, p.Next(s.now())) }()
			s.fire(ctx, p, at)
		}(plan, when)
	}
}

// claim reserves a schedule for this tick, or reports why not.
//
// Overlap is refused rather than queued: two bursts of the same statements
// racing each other deliver every customer two documents that disagree.
func (s *Service) claim(name string, now time.Time) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	when, armed := s.due[name]
	if !armed || now.Before(when) {
		return time.Time{}, false
	}
	if s.running[name] {
		s.log.Warn("schedule skipped — the previous run is still going",
			"schedule", name, "due", when.Format(time.RFC3339))
		return time.Time{}, false
	}
	s.running[name] = true
	return when, true
}

func (s *Service) release(name string, next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running[name] = false
	s.due[name] = next
}

// fire runs one schedule and records what happened.
func (s *Service) fire(ctx context.Context, p Plan, at time.Time) {
	start, end := p.Period(at)
	run := burst.Run{
		"periodStart": start.Format(time.DateOnly),
		"periodEnd":   end.Format(time.DateOnly),
		"periodLabel": Label(start, end),
		"period":      Label(start, end),
		"firedAt":     at.Format(time.RFC3339),
	}

	began := s.now()
	result, err := s.runner.Run(ctx, p.Schedule, run, s.owner.Owner(p.Schedule))
	took := s.now().Sub(began)

	if err != nil {
		s.log.Error("schedule failed", "schedule", p.Schedule.Name,
			"period", run["periodLabel"], "took", took, "err", err)
		return
	}
	// Partial success is logged as a failure, because it is one: a burst that
	// delivered 4,997 of 5,000 has three customers who did not get an invoice,
	// and an INFO line is where that goes to die.
	if len(result.Failed) > 0 {
		s.log.Error("schedule partially delivered", "schedule", p.Schedule.Name,
			"period", run["periodLabel"], "delivered", result.Delivered,
			"recipients", result.Recipients, "failed", len(result.Failed), "took", took)
		return
	}
	s.log.Info("schedule delivered", "schedule", p.Schedule.Name,
		"period", run["periodLabel"], "recipients", result.Recipients, "took", took)
}

// Due reports when each armed schedule next fires, for an operator asking why
// nothing has happened.
func (s *Service) Due() map[string]time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string]time.Time, len(s.due))
	for name, when := range s.due {
		out[name] = when
	}
	return out
}

// Check parses every schedule and reports the ones that will not arm.
//
// For a startup that should fail loudly rather than serve with two of its five
// schedules quietly missing.
func Check(src Source) error {
	for _, sched := range src.Schedules() {
		if _, err := Parse(sched); err != nil {
			return fmt.Errorf("schedule %q will not arm: %w", sched.Name, err)
		}
	}
	return nil
}
