package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
)

// The preflight is a promise about what the API answers, and a browser holds
// it to the letter: a method missing from the list is a request it refuses to
// send. That reaches the caller as a network failure — indistinguishable from
// an unreachable server, for a server that would have answered fine.
func TestThePreflightNamesEveryMethodTheAPIAnswers(t *testing.T) {
	h := api.NewCORS([]string{"http://localhost:5174"}, http.NotFoundHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/v1/definitions/Dataset/invoices", nil)
	req.Header.Set("Origin", "http://localhost:5174")
	h.ServeHTTP(rec, req)

	allowed := rec.Header().Get("Access-Control-Allow-Methods")
	/* This list grows every time a handler learns a verb, and twice now it has
	   grown late: DELETE when definitions became deletable, PATCH when people
	   became amendable. Both times the symptom was a request the browser
	   refused to send, reported as an unreachable server. */
	for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
		if !strings.Contains(allowed, method) {
			t.Errorf("%s is missing from %q, so a browser will not send one", method, allowed)
		}
	}
}

// A wildcard on an endpoint that reads an Authorization header means any page
// on the internet can make an authenticated request with a token it obtained —
// and embed tokens live in host pages, where they are obtainable.
func TestAnUnknownOriginIsToldNothing(t *testing.T) {
	h := api.NewCORS([]string{"http://localhost:5174"}, http.NotFoundHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/v1/definitions", nil)
	req.Header.Set("Origin", "https://not-ours.example")
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("an origin nobody allow-listed was allowed: %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Fatalf("and told what it could have called: %q", got)
	}
}

// Echoing the origin means the response varies by it, and a shared cache that
// misses this serves one customer's headers to another.
func TestTheResponseVariesByOrigin(t *testing.T) {
	h := api.NewCORS([]string{"http://localhost:5174"}, http.NotFoundHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog", nil)
	req.Header.Set("Origin", "http://localhost:5174")
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
		t.Fatalf("Vary is %q", rec.Header().Get("Vary"))
	}
}
