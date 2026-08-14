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
	source    Source
	runner    Runner
	owner     Owner
	log       *slog.Logger
	now       func() time.Time
	tickEvery time.Duration
	mu        sync.Mutex
	running   map[string]bool
	due       map[string]time.Time
	alerter   Alerter
	alerts    *alerts
	// grace is how long a burst already under way gets after the loop is told
	// to stop. See fireDue.
	grace time.Duration
	/*
	   ticked is when the loop last completed a pass.

	   The only evidence that this scheduler is working. Everything else a
	   deployment can measure counts things that happened — runs, deliveries,
	   failures — and a scheduler that has stopped produces none of them. Zero
	   failures is what a perfect night looks like and also what a dead loop
	   looks like, and no alert written against a counter can tell them apart.
	*/
	ticked time.Time
	/*
	   elector decides whether this process is the one that fires.

	   Absent means yes, which is every deployment that has not asked for
	   several replicas — and the file-backed one, which is a single process by
	   construction.
	*/
	elector Elector
	// leading is what the last election said, for the gauge. Cached rather
	// than asked at scrape time: a scrape should not open a database session.
	leading bool
}

/*
Elector says whether this process should be the one firing schedules.

A port rather than a mechanism, because the mechanism is the store's: leadership
is a Postgres advisory lock, and its liveness is the database's business rather
than this loop's. See sqlstore.Lease.
*/
type Elector interface {
	Leading(ctx context.Context) bool
}

// Tick is how often the loop looks for work.
//
// A minute, because cron's resolution is a minute. Anything finer is a busy
// loop asking the same question; anything coarser can miss a firing.
const Tick = time.Minute

/*
Grace is how long a burst already under way gets after the loop is told to stop.

Bounded, because the process is trying to exit and an orchestrator that sent
SIGTERM will send SIGKILL — thirty seconds later, by default. A grace longer
than the drain waiting on it is a grace that is never honoured.

Twenty seconds is one render and one delivery per remaining recipient at a rate
a deployment can be asked to sustain. A burst too large to finish in it is cut,
and the run record says so — which is the point: the record is the thing
somebody reconciles from.
*/
const Grace = 20 * time.Second

// WithGrace bounds how long in-flight bursts get after a stop.
func (s *Service) WithGrace(d time.Duration) *Service { s.grace = d; return s }

// New wires a Service.
func New(src Source, r Runner, o Owner, log *slog.Logger) *Service {
	return &Service{
		source: src, runner: r, owner: o, log: log,
		now: time.Now, tickEvery: Tick, grace: Grace,
		running: map[string]bool{},
		due:     map[string]time.Time{},
		alerts:  newAlerts(),
	}
}

/*
WithElection makes this one of several replicas, of which one fires.

Without it a scheduler fires whatever else is running, which is correct for one
process and is why CRONOS_SCHEDULER had to be set on exactly one instance — a
rule held in somebody's head, where setting it twice double-sends to every
customer and forgetting it sends to nobody.
*/
func (s *Service) WithElection(e Elector) *Service { s.elector = e; return s }

// Leading reports what the last election said, for the gauge that tells a
// deployment which replica is doing the work — and whether any is.
func (s *Service) Leading() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leading
}

// WithAlerts tells somebody when a run does not work.
//
// Optional, and absent means the failure reaches a log and no human — which is
// the state this replaces, and worth being able to see in a startup line.
func (s *Service) WithAlerts(a Alerter) *Service { s.alerter = a; return s }

// WithClock and WithTick make the loop testable without waiting for a minute
// of real time to pass.
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }
func (s *Service) WithTick(d time.Duration) *Service       { s.tickEvery = d; return s }

// Start runs until the context is cancelled.
//
// In-flight bursts finish. Killing one mid-render leaves a delivery that half
// happened, which is worse to reconcile than one that did not start.
func (s *Service) Start(ctx context.Context) error {
	// Before the first tick, so a scheduler that has just started does not look
	// like one that has been stuck since the epoch.
	s.tick()
	s.log.Info("scheduler started", "tick", s.tickEvery, "electing", s.elector != nil)

	ticker := time.NewTicker(s.tickEvery)
	defer ticker.Stop()

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("scheduler draining")
			return nil
		case <-ticker.C:
			/*
			   Elected on every pass, not once at startup.

			   Leadership is not a thing a process is given; it is a thing it
			   keeps having, and it can lose it — the database restarted, a
			   proxy dropped the session, this replica was partitioned away.
			   Asking each time is what makes a hand-over take one tick rather
			   than a deploy.
			*/
			if !s.elected(ctx) {
				// A follower arms nothing. Its Due() is empty, so the gauges
				// report zero schedules armed and nothing overdue — which is
				// the truth about a replica that is not scheduling, and stops
				// every follower firing the alert meant for a stuck leader.
				s.standDown()
				s.tick()
				continue
			}
			s.fireDue(ctx, &wg)
			s.tick()
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

			/*
			   A run does not inherit the loop's cancellation.

			   This is the whole of the shutdown guarantee, and it was the half
			   that was missing. Start blocks on these goroutines when the
			   context is cancelled — but every run was a child of that same
			   context, so cancelling to stop the loop also cancelled the work
			   the wait exists to protect. The wait then completed promptly,
			   because there was nothing left to wait for: eight hundred
			   recipients failed with "context canceled" in seventy
			   milliseconds, and twenty of them had a document.

			   Worse, the run record is written at the end and through the same
			   context, so it failed too. A burst that delivered to a fifth of a
			   customer list left no record of having run at all — which is
			   exactly the state nobody can reconcile, arrived at by the code
			   written to prevent it.

			   Bounded rather than detached: the process is exiting and the
			   grace must expire before the orchestrator's patience does. A
			   burst too large to finish is cut, and the record says so.
			*/
			runCtx, done := context.WithTimeout(context.WithoutCancel(ctx), s.grace)
			defer done()
			s.fire(runCtx, p, at)
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
		s.alert(ctx, p.Schedule, run["periodLabel"], Alert{Err: err.Error()}, true)
		return
	}
	// Partial success is logged as a failure, because it is one: a burst that
	// delivered 4,997 of 5,000 has three customers who did not get an invoice,
	// and an INFO line is where that goes to die.
	if len(result.Failed) > 0 {
		s.log.Error("schedule partially delivered", "schedule", p.Schedule.Name,
			"period", run["periodLabel"], "delivered", result.Delivered,
			"recipients", result.Recipients, "failed", len(result.Failed), "took", took)
		s.alert(ctx, p.Schedule, run["periodLabel"], Alert{
			Recipients: result.Recipients, Delivered: result.Delivered,
			Failures: result.Failed,
		}, true)
		return
	}
	s.log.Info("schedule delivered", "schedule", p.Schedule.Name,
		"period", run["periodLabel"], "recipients", result.Recipients, "took", took)
	s.alert(ctx, p.Schedule, run["periodLabel"], Alert{
		Recipients: result.Recipients, Delivered: result.Delivered,
	}, false)
}

// alert tells somebody, if there is somebody to tell and anything new to say.
func (s *Service) alert(ctx context.Context, sched definition.Schedule,
	period string, a Alert, failing bool) {

	if s.alerter == nil || sched.OnFailure.Alert == "" {
		return
	}

	s.mu.Lock()
	send, recovered := s.alerts.should(sched.Name, failing, s.now())
	s.mu.Unlock()

	if !send {
		return
	}
	a.To, a.Schedule, a.Period, a.Recovered = sched.OnFailure.Alert, sched.Name, period, recovered

	if err := s.alerter.Alert(ctx, a); err != nil {
		// Logged and swallowed. A burst that delivered is not undone because
		// the alert about it could not be sent, and an alerter that can fail a
		// run is a second thing that can take the run down.
		s.log.Error("alert not sent", "schedule", sched.Name, "to", a.To, "err", err)
	}
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

/*
elected asks whether this process should be firing, and says so when it changes.

Logged on the edge rather than every pass: a hand-over is worth a line and a
follower saying "still not me" once a minute is not.
*/
func (s *Service) elected(ctx context.Context) bool {
	now := true
	if s.elector != nil {
		now = s.elector.Leading(ctx)
	}

	s.mu.Lock()
	was := s.leading
	s.leading = now
	s.mu.Unlock()

	switch {
	case now && !was:
		s.log.Info("scheduling here now", "schedules", len(s.source.Schedules()))
	case !now && was:
		s.log.Info("another instance is scheduling")
	}
	return now
}

// standDown forgets what was armed, so a follower reports nothing armed and
// nothing overdue. What it was waiting for is recomputed from the schedules the
// moment it leads again — arm() is a pure function of now and the definitions.
func (s *Service) standDown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.due = map[string]time.Time{}
}

/*
tick records that the loop completed a pass.

LastTick below is what a deployment alerts on. The recording is here rather
than at the top of the loop so it means "a pass finished", not "a pass began" —
a loop wedged inside fireDue is exactly the case worth catching.
*/
func (s *Service) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ticked = s.now()
}

/*
LastTick is when this scheduler last completed a pass, or the zero time if it
has never run.

The signal that has no substitute. A process can serve every request, answer
health and readiness, and not be running anybody's schedules — because the flag
was never set, because Start returned early, because the goroutine died. From
outside, all three look like a quiet night.
*/
func (s *Service) LastTick() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ticked
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

// Fire runs one schedule now, as though its time had come.
//
// The only way to find out whether a monthly schedule works is to wait for the
// first of the month, unless something can run it deliberately. That is not a
// developer convenience: the recipients, the render and the delivery are the
// parts most likely to be wrong, and discovering it at 06:00 on the 1st is
// discovering it in front of the customer.
//
// It takes the same running lock as a due firing, so a manual run and a
// scheduled one cannot overlap for the same schedule. What it does not do is
// change when the schedule next fires: running it now is not a reason to skip
// the first of the month.
func (s *Service) Fire(ctx context.Context, name string) error {
	var found definition.Schedule
	for _, sched := range s.source.Schedules() {
		if sched.Name == name {
			found = sched
			break
		}
	}
	if found.Name == "" {
		return fmt.Errorf("%w: %q", ErrNoSchedule, name)
	}

	plan, err := Parse(found)
	if err != nil {
		return err
	}
	if !s.hold(name) {
		return fmt.Errorf("%w: %q is already running", ErrRunning, name)
	}
	defer s.unhold(name)

	s.fire(ctx, plan, s.now())
	return nil
}

// hold takes the running lock without touching when the schedule is next due.
func (s *Service) hold(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[name] {
		return false
	}
	if s.running == nil {
		s.running = map[string]bool{}
	}
	s.running[name] = true
	return true
}

func (s *Service) unhold(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running[name] = false
}
