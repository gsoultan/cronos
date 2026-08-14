package schedule_test

import (
	"context"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/app/schedule"
)

/*
A burst that is running when the process is asked to stop finishes.

This is the guarantee the whole shutdown path exists for, and the one most
likely to be lost: cronos is a report scheduler, so the work it exists to do
happens in these goroutines rather than in an HTTP handler. A rolling deploy at
six in the morning on the first of the month lands exactly on the monthly
statements burst.

Half a customer list receiving a document while the other half does not is the
worst state to be left in, because nobody can tell from outside which half —
and the run record, which is the only thing that could say, is written at the
end.

Start already held a WaitGroup over its in-flight runs and blocked on it. What
made that unreachable was above it: boot launched these goroutines and kept no
handle, so the process cancelled, returned, and the runtime tore down the
goroutine doing the waiting. This test pins the half that lives here; the boot
side is pinned by drainSchedulers.
*/
func TestStartWaitsForARunInFlight(t *testing.T) {
	c := &clock{t: at("2026-07-15T12:00:00Z")}
	held := make(chan struct{})
	r := &runner{block: held}

	svc := schedule.New(source{monthly()}, r, owner{}, quiet()).
		WithClock(c.now).WithTick(5 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { defer close(stopped); _ = svc.Start(ctx) }()

	// Armed first: arm() computes the next firing from whatever `now` says, so
	// moving the clock before it has armed simply arms it a month later and
	// nothing ever fires.
	armed(t, svc, 1)

	// Move the clock past the firing so a run is in flight and blocked.
	c.set(at("2026-08-01T05:00:00Z")) // past 06:00 Berlin
	waitFor(t, func() bool { return r.count() == 1 }, "the run to start")

	// SIGTERM, in effect.
	cancel()

	select {
	case <-stopped:
		t.Fatal("the scheduler returned while a burst was still delivering")
	case <-time.After(150 * time.Millisecond):
		// Correct: still waiting on the run.
	}

	close(held) // the burst finishes
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the scheduler never returned after its run finished")
	}
}

// And a scheduler with nothing in flight stops immediately. A drain that
// always took its full bound would make every deploy wait for the worst case.
func TestStartReturnsAtOnceWhenNothingIsRunning(t *testing.T) {
	c := &clock{t: at("2026-07-15T12:00:00Z")}
	svc := schedule.New(source{monthly()}, &runner{}, owner{}, quiet()).
		WithClock(c.now).WithTick(5 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { defer close(stopped); _ = svc.Start(ctx) }()

	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("an idle scheduler did not stop")
	}
}

func waitFor(t *testing.T, ok func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

/*
A run already under way is not cancelled by the stop that waits for it.

This is the half that made the wait useless, and it is the one a unit test with
a blocked runner cannot see: a runner sitting on a channel is not a runner
whose context has been cancelled, so Start "waited" while every render inside
it had already been told to abort.

A live burst of eight hundred said what really happened. Twenty had a document,
seven hundred and eighty failed with "context canceled" in seventy
milliseconds, and the run record — written at the end, through the same context
— failed too. A burst that delivered to a fortieth of a customer list left no
record of having run at all: exactly the state nobody can reconcile, reached by
the code written to prevent it.
*/
func TestARunInFlightKeepsALiveContext(t *testing.T) {
	c := &clock{t: at("2026-07-15T12:00:00Z")}
	seen := make(chan error, 1)
	r := &runner{watch: seen}

	svc := schedule.New(source{monthly()}, r, owner{}, quiet()).
		WithClock(c.now).WithTick(5 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { defer close(stopped); _ = svc.Start(ctx) }()

	armed(t, svc, 1)
	c.set(at("2026-08-01T05:00:00Z"))
	waitFor(t, func() bool { return r.count() == 1 }, "the run to start")

	// SIGTERM while it is running.
	cancel()

	select {
	case err := <-seen:
		if err != nil {
			t.Fatalf("the run was cancelled by the stop meant to wait for it: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the run never reported what its context said")
	}

	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("the scheduler never returned")
	}
}

// And the grace is bounded, so a burst that will not finish cannot hold the
// process past the patience of whatever sent SIGTERM.
func TestTheRunGraceIsBounded(t *testing.T) {
	if schedule.Grace <= 0 {
		t.Fatal("a run in flight has no bound at all")
	}
	if schedule.Grace >= 30*time.Second {
		t.Fatalf("the grace (%s) outlives a default SIGKILL", schedule.Grace)
	}
}

/*
The loop says it is going round.

The one signal with no substitute. A process can serve every request, answer
health and readiness, and run nobody's schedules — because the flag was never
set, because Start returned early, because the goroutine died. All three look
identical from outside, and all three look identical to every counter this
product had, because a scheduler that is not running produces no runs to count.

Recorded when a pass finishes rather than when one begins, so a loop wedged
inside a firing is caught rather than reported as healthy.
*/
func TestTheLoopRecordsItsPasses(t *testing.T) {
	c := &clock{t: at("2026-07-15T12:00:00Z")}
	svc := schedule.New(source{monthly()}, &runner{}, owner{}, quiet()).
		WithClock(c.now).WithTick(5 * time.Millisecond)

	if !svc.LastTick().IsZero() {
		t.Fatal("a scheduler that has never started claims to have ticked")
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { defer close(stopped); _ = svc.Start(ctx) }()
	t.Cleanup(func() { cancel(); <-stopped })

	waitFor(t, func() bool { return !svc.LastTick().IsZero() }, "the first pass")

	// And it keeps saying so. A clock that only moves when the test moves it
	// makes this exact rather than approximate.
	first := svc.LastTick()
	c.set(at("2026-07-15T12:00:30Z"))
	waitFor(t, func() bool { return svc.LastTick().After(first) }, "a later pass")
}
