package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
)

// The exposition has to be the format everything already scrapes, and the
// labels have to be bounded. Both are easy to get almost right.
func TestTheExpositionIsScrapableAndBounded(t *testing.T) {
	m := api.NewMetrics()
	m.Request("/v1/reports/{name}", 200, 120*time.Millisecond)
	m.Request("/v1/reports/{name}", 200, 900*time.Millisecond)
	m.Request("/v1/reports/{name}", 404, 3*time.Millisecond)
	m.Run("partial", 812, 810)

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/metrics", nil))
	body := rec.Body.String()

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content type %q", ct)
	}
	for _, want := range []string{
		`cronos_requests_total{route="/v1/reports/{name}",status="200"} 2`,
		`cronos_requests_total{route="/v1/reports/{name}",status="404"} 1`,
		`cronos_request_duration_seconds_count{route="/v1/reports/{name}"} 3`,
		`cronos_runs_total{result="partial"} 1`,
		// Counted rather than derived: "how many customers did not get theirs"
		// is the alert, and making somebody subtract two series to ask it is
		// how the alert does not get written.
		`cronos_deliveries_total{result="failed"} 2`,
		`cronos_deliveries_total{result="delivered"} 810`,
		"# TYPE cronos_requests_total counter",
		"# TYPE cronos_request_duration_seconds histogram",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing:\n  %s\ngot:\n%s", want, body)
		}
	}
}

// Buckets are cumulative in this format: a 120ms request counts in every
// bucket at or above it. Getting this wrong produces a histogram that renders
// and is wrong, which is worse than one that does not render.
func TestBucketsAreCumulative(t *testing.T) {
	m := api.NewMetrics()
	m.Request("/x", 200, 120*time.Millisecond)

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/metrics", nil))
	body := rec.Body.String()

	for _, want := range []string{
		`cronos_request_duration_seconds_bucket{route="/x",le="0.1"} 0`,
		`cronos_request_duration_seconds_bucket{route="/x",le="0.25"} 1`,
		`cronos_request_duration_seconds_bucket{route="/x",le="+Inf"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in:\n%s", want, body)
		}
	}
}

// A path carries a report name. One series per report is one series per
// customer of our customer — a cardinality explosion, and in a shared
// monitoring system a list of who they sell to.
func TestAnUnmatchedPathDoesNotBecomeItsOwnSeries(t *testing.T) {
	counted := api.NewMetrics()
	h := api.NewObserved(http.NotFoundHandler(), logger(&bytes.Buffer{})).WithMetrics(counted)

	for _, path := range []string{"/wp-admin", "/.env", "/api/v2/whatever"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	rec := httptest.NewRecorder()
	counted.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/metrics", nil))
	body := rec.Body.String()

	if !strings.Contains(body, `cronos_requests_total{route="unmatched",status="404"} 3`) {
		t.Fatalf("scanned paths were not folded together:\n%s", body)
	}
	for _, path := range []string{"wp-admin", ".env"} {
		if strings.Contains(body, path) {
			t.Errorf("%q became a series", path)
		}
	}
}
