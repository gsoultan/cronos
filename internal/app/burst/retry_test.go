package burst

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// nowait replaces the backoff so a ladder does not cost real seconds.
func nowait(waited *[]time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error {
		*waited = append(*waited, d)
		return nil
	}
}

func TestATransientFailureIsRetried(t *testing.T) {
	var waited []time.Duration
	calls := 0

	attempts, err := attempt(context.Background(),
		definition.FailureSpec{Retries: 3}, nowait(&waited), func() error {
			calls++
			if calls < 3 {
				return errors.New("connection refused")
			}
			return nil
		})

	if err != nil {
		t.Fatalf("it should have succeeded on the third: %v", err)
	}
	if attempts != 3 || calls != 3 {
		t.Errorf("attempts = %d, calls = %d", attempts, calls)
	}
	// Exponential by default, and the record says how many it took.
	if len(waited) != 2 || waited[0] != BaseBackoff || waited[1] != 2*BaseBackoff {
		t.Errorf("waited %v", waited)
	}
}

// Retrying an address that does not exist is three more rejections, three more
// delays, and a burst that takes an hour longer to reach the same answer.
func TestAPermanentFailureIsNotRetried(t *testing.T) {
	var waited []time.Duration
	calls := 0

	attempts, err := attempt(context.Background(),
		definition.FailureSpec{Retries: 5}, nowait(&waited), func() error {
			calls++
			return Fatal(errors.New("no such mailbox"))
		})

	if err == nil {
		t.Fatal("want an error")
	}
	if calls != 1 || attempts != 1 {
		t.Errorf("tried %d times", calls)
	}
	if len(waited) != 0 {
		t.Errorf("it waited: %v", waited)
	}
	// The cause survives, so a delivery record says what actually happened.
	if !strings.Contains(err.Error(), "no such mailbox") {
		t.Errorf("err = %v", err)
	}
}

func TestGivingUpSaysHowManyItTried(t *testing.T) {
	var waited []time.Duration
	_, err := attempt(context.Background(),
		definition.FailureSpec{Retries: 2}, nowait(&waited), func() error {
			return errors.New("503")
		})

	if err == nil || !strings.Contains(err.Error(), "after 3 attempts") {
		t.Errorf("err = %v", err)
	}
}

// Retries multiply: ten recipients with ten retries each is a hundred attempts
// against a service that has already said no.
func TestRetriesAreCapped(t *testing.T) {
	var waited []time.Duration
	calls := 0
	attempt(context.Background(), definition.FailureSpec{Retries: 500},
		nowait(&waited), func() error { calls++; return errors.New("nope") })

	if calls != MaxRetries+1 {
		t.Errorf("tried %d times, want the cap of %d", calls, MaxRetries+1)
	}
}

// An unbounded exponential reaches numbers where the schedule's next firing
// arrives before the current one finishes.
func TestBackoffIsBounded(t *testing.T) {
	for n := 1; n < 20; n++ {
		if got := backoff("exponential", n); got > MaxBackoff {
			t.Fatalf("attempt %d waits %s", n, got)
		}
	}
	if got := backoff("constant", 4); got != BaseBackoff {
		t.Errorf("constant backoff grew to %s", got)
	}
	if got := backoff("none", 2); got != 0 {
		t.Errorf("none waited %s", got)
	}
}

// A shutdown must not be held for the length of a backoff ladder.
func TestCancellationInterruptsTheWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	began := time.Now()
	_, err := attempt(ctx, definition.FailureSpec{Retries: 5}, pause,
		func() error { return errors.New("try again") })

	if err == nil {
		t.Fatal("want an error")
	}
	if time.Since(began) > time.Second {
		t.Errorf("waited %s for a cancelled run", time.Since(began))
	}
}
