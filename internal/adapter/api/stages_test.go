package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
)

/*
Where a scheduled run's time went.

The request histogram cannot answer it. A burst is started by the scheduler, so
there is no request to time, and until this existed the only thing counted
about a four-hour burst was that it finished — one number with three candidates
behind it, and the two that matter have their fixes in different places:
rendering is this machine and the typesetter, delivery is somebody else's SMTP
server.
*/

func scraped(t *testing.T, m *api.Metrics) string {
	t.Helper()

	w := httptest.NewRecorder()
	m.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("scrape answered %d", w.Code)
	}
	return w.Body.String()
}

func TestAStageIsExposedAsAHistogram(t *testing.T) {
	m := api.NewMetrics()
	m.Stage("render", 120*time.Millisecond)
	m.Stage("render", 300*time.Millisecond)
	m.Stage("deliver", 2*time.Second)

	out := scraped(t, m)

	for _, want := range []string{
		"# TYPE cronos_stage_duration_seconds histogram",
		`cronos_stage_duration_seconds_count{stage="render"} 2`,
		`cronos_stage_duration_seconds_count{stage="deliver"} 1`,
		// The bucket a 120ms and a 300ms render both fall under.
		`cronos_stage_duration_seconds_bucket{stage="render",le="0.5"} 2`,
		`cronos_stage_duration_seconds_bucket{stage="render",le="+Inf"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the exposition is missing:\n  %s", want)
		}
	}
}

// Generalising histogram.write must not have moved the request metric, which
// dashboards and alerts already name.
func TestTheRequestHistogramIsUnchanged(t *testing.T) {
	m := api.NewMetrics()
	m.Request("/v1/reports/:name", http.StatusOK, 250*time.Millisecond)

	out := scraped(t, m)

	for _, want := range []string{
		"# TYPE cronos_request_duration_seconds histogram",
		`cronos_request_duration_seconds_count{route="/v1/reports/:name"} 1`,
		`cronos_request_duration_seconds_bucket{route="/v1/reports/:name",le="0.25"} 1`,
		`cronos_request_duration_seconds_sum{route="/v1/reports/:name"} 0.25`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the request histogram changed shape, missing:\n  %s", want)
		}
	}
}

// A deployment that has run no bursts exposes the metric with no series rather
// than omitting it, so a dashboard querying it reads empty instead of broken.
func TestTheStageMetricIsDeclaredBeforeAnyRunHasHappened(t *testing.T) {
	out := scraped(t, api.NewMetrics())

	if !strings.Contains(out, "# TYPE cronos_stage_duration_seconds histogram") {
		t.Error("the stage metric is not declared until something is observed")
	}
	if strings.Contains(out, `cronos_stage_duration_seconds_count{stage=`) {
		t.Error("a fresh instance reports stage observations it cannot have")
	}
}

// Concurrent bursts record concurrently. A burst renders on a bounded fan-out,
// so every one of these calls arrives from a different goroutine.
func TestStagesRecordUnderConcurrency(t *testing.T) {
	m := api.NewMetrics()

	done := make(chan struct{})
	for range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			for range 50 {
				m.Stage("render", time.Millisecond)
			}
		}()
	}
	for range 8 {
		<-done
	}

	if want := `cronos_stage_duration_seconds_count{stage="render"} 400`; !strings.Contains(scraped(t, m), want) {
		t.Errorf("lost observations under concurrency, wanted:\n  %s", want)
	}
}
