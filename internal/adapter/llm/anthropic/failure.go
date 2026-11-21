package anthropic

import (
	"context"
	"errors"
	"fmt"
)

// Failure is the provider refusing or failing a request, with what is safe
// to log.
type Failure struct {
	// Status is the HTTP status the API answered, or zero when it was not
	// reached.
	Status int
	Cause  error
}

func (f *Failure) Error() string {
	if f.Status != 0 {
		return fmt.Sprintf("anthropic: status %d", f.Status)
	}
	if errors.Is(f.Cause, context.DeadlineExceeded) {
		return "anthropic: timed out"
	}
	return "anthropic: unreachable"
}

func (f *Failure) Unwrap() error { return f.Cause }
