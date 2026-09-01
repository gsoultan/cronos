package publish_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
A publish that is abandoned stops talking to the warehouse.

Proving a report means preparing every one of its blocks against the database
it will read — a report with four outputs of six blocks is twenty-four
statements on somebody else's warehouse. That step used to run on a fresh
context.Background(), so it was the one step of a publish that could not be
cancelled, and it was the only one talking to a database this deployment does
not own.

A browser tab closed halfway through left the rest to run to their own
statement timeout with nobody waiting for the answer. Bounded, so never a leak
— but on a warehouse with a connection limit, work continuing after the last
interested party has gone is somebody else's incident.

The test asserts the property directly: cancel before publishing, and the
verifier must see a cancelled context rather than a live one.
*/

// seenCtx is an Engines whose Verifier records the context it was handed.
type seenCtx struct{ err error }

func (s *seenCtx) Engine(context.Context, definition.Dataset) (run.Engine, error) {
	return run.Engine{Executor: s, Builder: query.NewBuilder(query.Postgres{})}, nil
}

func (s *seenCtx) Execute(context.Context, query.Plan) (run.Rows, error) {
	return nil, errors.New("not called")
}

func (s *seenCtx) Verify(ctx context.Context, _ query.Plan) error {
	// The whole assertion. A Background context has no Done channel at all, so
	// this is not merely "not yet cancelled" — it is a context that can never
	// be cancelled by anybody.
	s.err = ctx.Err()
	if ctx.Done() == nil {
		s.err = errors.New("the verifier was handed a context nobody can cancel")
	}
	return nil
}

func TestAnAbandonedPublishStopsProvingBlocks(t *testing.T) {
	svc, _, _ := setup(t)
	engines := &seenCtx{}
	svc = svc.WithEngines(engines)

	mustPublish(t, svc, dataset)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The publish itself may fail on the cancelled context, and that is fine:
	// what matters is what the verifier saw if it ran at all.
	_, _ = svc.Publish(ctx, []byte(report), admin())

	if engines.err == nil {
		t.Fatal("the verifier ran on a context that was not the caller's")
	}
	if !errors.Is(engines.err, context.Canceled) {
		t.Fatalf("the verifier saw %v, not the caller's cancellation", engines.err)
	}
}
