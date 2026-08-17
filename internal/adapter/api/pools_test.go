package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type pool struct {
	open, inUse, limit int
}

func (p pool) Names() []string { return []string{"warehouse"} }
func (p pool) Pool(string) (int, int, int, bool) {
	return p.open, p.inUse, p.limit, true
}

/*
The scrape has to finish.

writePools took m.mu, and ServeHTTP already holds it for the whole exposition.
sync.Mutex is not reentrant, so every scrape deadlocked — and the symptom was
the worst kind: the endpoint accepted the connection and never answered, while
liveness and readiness stayed green and the process looked perfectly healthy. A
monitoring system would have shown a gap in the graphs and nothing else.

So this is a timeout, not a correctness check. It is the one thing a test can
say about a deadlock.
*/
func TestScrapingWithPoolsRegisteredDoesNotDeadlock(t *testing.T) {
	m := NewMetrics()
	m.WatchPools("acme/finance", pool{open: 3, inUse: 1, limit: 8})

	done := make(chan string, 1)
	go func() {
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/metrics", nil))
		done <- rec.Body.String()
	}()

	select {
	case body := <-done:
		for _, want := range []string{
			`cronos_datasource_connections{project="acme/finance",source="warehouse"} 3`,
			`cronos_datasource_connections_in_use{project="acme/finance",source="warehouse"} 1`,
			`cronos_datasource_connections_limit{project="acme/finance",source="warehouse"} 8`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("the exposition does not contain:\n  %s", want)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the scrape never finished — /v1/metrics accepts the connection and hangs, " +
			"which health and readiness cannot see")
	}
}

/*
Every sample of a metric follows its own HELP and TYPE.

Three numbers per source written source-by-source rather than family-by-family
is a scrape some collectors reject and others silently keep only the last of.
The bug is invisible with one source and appears with two.
*/
func TestPoolSamplesAreGroupedByFamily(t *testing.T) {
	m := NewMetrics()
	m.WatchPools("acme/finance", pool{open: 3, inUse: 1, limit: 8})
	m.WatchPools("globex/ops", pool{open: 5, inUse: 2, limit: 16})

	var out strings.Builder
	m.writePools(&out)

	for _, family := range []string{
		"cronos_datasource_connections",
		"cronos_datasource_connections_in_use",
		"cronos_datasource_connections_limit",
	} {
		if n := strings.Count(out.String(), "# TYPE "+family+" "); n != 1 {
			t.Fatalf("%s is declared %d times", family, n)
		}
	}

	// Both projects' samples of the first family come before the second
	// family is declared.
	body := out.String()
	firstDecl := strings.Index(body, "# TYPE cronos_datasource_connections_in_use ")
	for _, project := range []string{"acme/finance", "globex/ops"} {
		at := strings.Index(body, `cronos_datasource_connections{project="`+project+`"`)
		if at < 0 || at > firstDecl {
			t.Fatalf("%s's sample of the first family is not under its own declaration", project)
		}
	}
}

// A deployment with no pools registered writes nothing, rather than a header
// with no samples under it.
func TestNoPoolsWritesNothing(t *testing.T) {
	var out strings.Builder
	NewMetrics().writePools(&out)
	if out.Len() != 0 {
		t.Fatalf("wrote %q with nothing registered", out.String())
	}
}
