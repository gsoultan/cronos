package boot

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/platform/config"
)

/*
Shutdown waits for scheduled runs, and gives up rather than hanging.

The other half of the guarantee in app/schedule, where Start blocks on its
in-flight bursts. That guarantee was unreachable: boot launched the scheduler
goroutines and kept no handle on them, so the process cancelled, returned, and
the runtime tore down the goroutine that was doing the waiting. The drain
written to let a burst finish was the thing that killed it.

cronos is a report scheduler, so this is the path the product exists for. A
rolling deploy at six in the morning on the first of the month lands exactly on
the monthly statements burst, and half a customer list receiving a document
while the other half does not is the worst state to be left in — nobody can
tell from outside which half, and the run record that could say is written at
the end.
*/
func TestShutdownWaitsForScheduledRuns(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	finished := make(chan struct{})
	go func() { defer wg.Done(); <-finished }()

	returned := make(chan struct{})
	go func() { defer close(returned); drain(wg.Wait, quietLog()) }()

	select {
	case <-returned:
		t.Fatal("shutdown returned while a burst was still delivering")
	case <-time.After(120 * time.Millisecond):
	}

	close(finished)
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown never returned after the burst finished")
	}
}

/*
And it gives up at the bound.

A scheduler that will not stop must not be the reason an orchestrator escalates
SIGTERM to SIGKILL: that kills the run anyway and takes the record of it with
it, which is strictly worse than stopping deliberately and saying so.
*/
func TestTheDrainIsBoundedBelowTheGracePeriod(t *testing.T) {
	if schedulerDrain <= 0 {
		t.Fatal("the drain has no bound at all")
	}
	// Kubernetes sends SIGKILL thirty seconds after SIGTERM by default.
	if schedulerDrain >= 30*time.Second {
		t.Fatalf("the drain (%s) outlives the default grace period", schedulerDrain)
	}
}

// A deployment with no scheduler stops at once rather than waiting for one.
func TestShutdownWithNoSchedulersIsImmediate(t *testing.T) {
	done := make(chan struct{})
	go func() { defer close(done); drain(nil, quietLog()) }()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a deployment with no scheduler waited for one")
	}
}

/*
Stopping is one call, so a caller cannot cancel without waiting.

This used to be two — a context cancel held by boot, and a wait that existed
only inside each scheduler — and it was the wait that went missing. The shape
here is what makes that particular mistake unwritable: there is nothing to call
that cancels and does not wait.
*/
func TestStoppingSchedulersAlsoWaitsForThem(t *testing.T) {
	// No scheduler configured, so this exercises the returned stop itself
	// rather than any project's loop: it must still be safe and immediate.
	stop, err := startSchedulers(config.Server{Scheduler: false}, nil, nil, nil, nil, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	if stop == nil {
		t.Fatal("nothing to stop the schedulers with")
	}
	done := make(chan struct{})
	go func() { defer close(done); stop() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stopping a deployment with no scheduler blocked")
	}
}

func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
