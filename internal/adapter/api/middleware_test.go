package api_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
)

// A panic in one handler must not take the process with it.
//
// Without this, a nil map anywhere ends a process that is often halfway
// through delivering five thousand documents, each of which is somebody's
// invoice — and the next request finds nothing listening.
func TestAPanicBecomesAnAnswerRatherThanAnExit(t *testing.T) {
	var logged bytes.Buffer
	h := api.NewObserved(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		var m map[string]string
		m["boom"] = "" // the classic
	}), logger(&logged))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/reports/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	// The stack goes to the log and never to the caller: a panic names a file
	// path and sometimes a query, and the caller is somebody else's customer.
	if strings.Contains(rec.Body.String(), "middleware_test.go") {
		t.Fatalf("the stack reached the caller: %s", rec.Body.String())
	}
	if !strings.Contains(logged.String(), "panic serving request") {
		t.Fatal("nothing was logged about it")
	}
	if !strings.Contains(logged.String(), "middleware_test.go") {
		t.Fatal("the log has no stack, which is the only reason to catch it here")
	}
}

// The id is quotable. A customer saying "it was wrong at 06:12" and a log line
// are otherwise two facts with nothing joining them.
func TestTheAnswerCarriesAnIDAndSoDoesTheLog(t *testing.T) {
	var logged bytes.Buffer
	h := api.NewObserved(ok(), logger(&logged))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/catalog", nil))

	id := rec.Header().Get("X-Request-Id")
	if id == "" {
		t.Fatal("no id came back")
	}
	if !strings.Contains(logged.String(), id) {
		t.Fatalf("the log does not mention %q", id)
	}
}

// A load balancer or a host that already stamped one is describing the same
// request. Minting a second breaks the chain at the boundary somebody is
// trying to trace across.
func TestACallersOwnIDIsKept(t *testing.T) {
	h := api.NewObserved(ok(), logger(&bytes.Buffer{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog", nil)
	req.Header.Set("X-Request-Id", "edge-7f3a")
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-Id"); got != "edge-7f3a" {
		t.Fatalf("id is %q", got)
	}
}

// But not one that would land in a log line unescaped, or one long enough to
// be a payload rather than an identifier.
func TestAnImplausibleIDIsReplaced(t *testing.T) {
	for _, given := range []string{
		"has space", "new\nline", strings.Repeat("x", 65), `"quoted"`, "semi;colon",
	} {
		h := api.NewObserved(ok(), logger(&bytes.Buffer{}))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/catalog", nil)
		req.Header.Set("X-Request-Id", given)
		h.ServeHTTP(rec, req)

		if got := rec.Header().Get("X-Request-Id"); got == given {
			t.Errorf("%q was accepted as an id", given)
		}
	}
}

// A refused request is not a server problem. Logging every wrong password at
// error level is how a real error gets missed.
func TestTheLevelReflectsWhatHappened(t *testing.T) {
	for status, want := range map[int]string{
		http.StatusOK:                  "level=INFO",
		http.StatusUnauthorized:        "level=WARN",
		http.StatusInternalServerError: "level=ERROR",
	} {
		var logged bytes.Buffer
		h := api.NewObserved(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}), logger(&logged))

		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/x", nil))
		if !strings.Contains(logged.String(), want) {
			t.Errorf("status %d logged as %q, want %s", status, logged.String(), want)
		}
	}
}

// A handler that panicked after writing has already sent a status. Writing a
// second one produces a log line about superfluous headers rather than a fix.
func TestAPanicAfterWritingDoesNotWriteAgain(t *testing.T) {
	h := api.NewObserved(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"partial":`))
		panic("halfway")
	}), logger(&bytes.Buffer{}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/x", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("the status was rewritten to %d", rec.Code)
	}
}

func ok() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}

func logger(into *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(into, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
