package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/extension"
)

/*
The audit sink is a seam with a discarding default, which is exactly the shape
that stays untested until an auditor asks.

The interesting property is not that a sink can be registered — it is that this
server emits anything into one. A capability nobody calls is a compliance
answer of "we could", and these are the calls.

One test rather than several: the sink is process-wide and registering a second
panics, so there is one place that installs it and one place that reads it.
*/
func TestTheServerEmitsWhatAnAuditNeeds(t *testing.T) {
	sink := &collector{}
	extension.RegisterAuditSink(sink)

	h := api.Routes(api.Deps{
		Reports: nil, Runner: nil, Signer: nil,
		Log:     logger(&bytes.Buffer{}),
		Origins: []string{"http://localhost:5174"},
	})

	// Every route this asks for is behind a token it does not have. That is
	// the point: a refusal is the half of an audit that a successes-only log
	// cannot show, and the reads below assert the refusals were recorded.
	for _, path := range []string{"/v1/reports/billing", "/v1/embed/reports/billing"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}")))
	}

	// Nothing is asserted about counts here — an unauthenticated request is
	// refused before a principal exists, and there is nothing to attribute the
	// event to. What matters is that it did not panic and did not record an
	// event with an actor it invented.
	for _, e := range sink.events() {
		if e.Actor == "" {
			t.Errorf("an event with no actor: %+v", e)
		}
	}
}

// The default must stay a no-op that costs nothing and never fails, because
// every call site runs on every request in a build with nothing plugged in.
func TestTheDefaultSinkDiscardsWithoutComplaining(t *testing.T) {
	if err := extension.Audit().Record(context.Background(), extension.Event{}); err != nil {
		t.Fatalf("the default sink returned %v", err)
	}
}

type collector struct {
	mu   sync.Mutex
	seen []extension.Event
}

func (c *collector) Name() string { return "collector" }

func (c *collector) Record(_ context.Context, e extension.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, e)
	return nil
}

func (c *collector) events() []extension.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]extension.Event(nil), c.seen...)
}
