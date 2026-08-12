package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
)

/*
Liveness and readiness answer different questions, and answering only the first
is how a load balancer keeps sending work to a process whose warehouse has gone
away.

The three states are the point. Down means send nothing. Degraded means one of
four warehouses is unreachable, three-quarters of the reports still work, and
taking the instance out of rotation would fail those too.
*/
func TestARequiredDependencyDownMeansDoNotSendWork(t *testing.T) {
	h := api.NewReady(logger(&bytes.Buffer{}), api.Check{
		Name:     "store",
		Required: true,
		Probe:    func(context.Context) error { return errors.New("connection refused") },
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503 — a load balancer reads the code", rec.Code)
	}
	answer := decodeReady(t, rec)
	if answer["status"] != "down" {
		t.Fatalf("status %q", answer["status"])
	}
	// Named, so whoever reads a failed probe knows which thing to go and look
	// at rather than which process to restart.
	checks := answer["checks"].(map[string]any)
	if checks["store"] != "connection refused" {
		t.Fatalf("the check does not say what happened: %v", checks)
	}
}

func TestAnOptionalDependencyDownIsStillServed(t *testing.T) {
	h := api.NewReady(logger(&bytes.Buffer{}),
		api.Check{Name: "store", Required: true, Probe: func(context.Context) error { return nil }},
		api.Check{Name: "datasource:archive", Probe: func(context.Context) error {
			return errors.New("no route to host")
		}},
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/ready", nil))

	// 200, because the reports that do not read the archive still work and
	// taking this instance out of rotation would fail those too.
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if got := decodeReady(t, rec)["status"]; got != "degraded" {
		t.Fatalf("status %q, want degraded", got)
	}
}

/*
The probes are cached briefly, and this is why.

A readiness probe every second from each of several load balancers, each
opening a connection to every warehouse, is a denial of service this product
performs on its own customers' databases.
*/
func TestProbesAreNotRunOncePerRequest(t *testing.T) {
	var probes atomic.Int64
	h := api.NewReady(logger(&bytes.Buffer{}), api.Check{
		Name: "store",
		Probe: func(context.Context) error {
			probes.Add(1)
			return nil
		},
	})

	for i := 0; i < 20; i++ {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/ready", nil))
	}
	if n := probes.Load(); n != 1 {
		t.Fatalf("%d probes for 20 requests — each one reaches a customer's database", n)
	}
}

// A deployment with nothing to ask is ready, rather than answering a question
// it has no way to answer.
func TestNoChecksIsReady(t *testing.T) {
	h := api.NewReady(logger(&bytes.Buffer{}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func decodeReady(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return out
}
