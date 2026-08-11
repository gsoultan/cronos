package burst

import (
	"context"
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// BaseBackoff is the first wait between attempts.
//
// A second, because the failures worth retrying are a relay refusing a
// connection or an object store returning 503, and both clear in seconds.
// Retrying faster is a denial of service aimed at somebody who is already
// having a bad morning.
const BaseBackoff = time.Second

// MaxBackoff caps the wait however many attempts are configured.
//
// A burst has five thousand of these to get through. An unbounded exponential
// reaches numbers where the schedule's next firing arrives before the current
// one finishes.
const MaxBackoff = 30 * time.Second

// MaxRetries bounds what a definition may ask for.
//
// Retries multiply: ten recipients failing with ten retries each is a hundred
// attempts against a service that has already said no.
const MaxRetries = 5

// attempt runs deliver until it succeeds, gives up, or is told not to bother.
//
// Returns the number of attempts made, so a delivery record can say three
// rather than say "failed" and leave somebody guessing whether it was tried.
func attempt(ctx context.Context, policy definition.FailureSpec,
	sleep func(context.Context, time.Duration) error, deliver func() error) (int, error) {

	tries := policy.Retries + 1
	if tries > MaxRetries+1 {
		tries = MaxRetries + 1
	}
	if tries < 1 {
		tries = 1
	}

	var err error
	for n := 1; n <= tries; n++ {
		if err = deliver(); err == nil {
			return n, nil
		}
		if permanent(err) {
			// An address that does not exist will not start existing. Reported
			// with the attempt count so far, which is one.
			return n, err
		}
		if n == tries {
			break
		}
		if wait := backoff(policy.Backoff, n); wait > 0 {
			if err := sleep(ctx, wait); err != nil {
				return n, err
			}
		}
	}
	return tries, fmt.Errorf("after %d attempts: %w", tries, err)
}

// backoff is how long to wait before attempt n+1.
//
// Exponential unless the definition asked for constant. No jitter: a burst's
// concurrency is already bounded, so the thundering herd this would smooth is
// eight workers rather than five thousand — and a deterministic wait is one an
// operator can predict from a log.
func backoff(kind string, attempt int) time.Duration {
	if kind == "none" {
		return 0
	}
	wait := BaseBackoff
	if kind != "constant" {
		for range attempt - 1 {
			wait *= 2
			if wait >= MaxBackoff {
				return MaxBackoff
			}
		}
	}
	if wait > MaxBackoff {
		return MaxBackoff
	}
	return wait
}

// pause waits, or gives up early if the run is being cancelled.
//
// A shutdown must not be held for the length of a backoff ladder. The
// in-flight delivery finishes; the wait before the next attempt does not.
func pause(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
