package burst

import (
	"context"

	"github.com/gsoultan/cronos/internal/core/history"
)

// Recorder writes down what happened.
//
// Three calls rather than one at the end: a run is recorded when it starts, so
// a burst that crashed halfway is still a run somebody can look at. One that
// only exists once it succeeded is a log of successes.
//
// Recording must never fail a delivery. A document that reached a customer has
// reached them whatever the audit table thinks, and unwinding that because a
// write failed would be a worse outcome than an incomplete record.
type Recorder interface {
	Begin(ctx context.Context, r history.Run) error
	Delivered(ctx context.Context, d history.Delivery) error
	Finish(ctx context.Context, id string, r history.Run) error
}

// discard is the recorder used when none is configured.
//
// A no-op rather than a nil check at four call sites: the check would be
// forgotten once, and the panic would be in the middle of a burst.
type discard struct{}

func (discard) Begin(context.Context, history.Run) error          { return nil }
func (discard) Delivered(context.Context, history.Delivery) error { return nil }
func (discard) Finish(context.Context, string, history.Run) error { return nil }
