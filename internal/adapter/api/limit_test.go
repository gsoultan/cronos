package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
)

// The burst is spent, then the rate applies.
func TestABurstIsAllowedAndThenIsNot(t *testing.T) {
	l := api.NewLimit(1, 3)

	for i := 0; i < 3; i++ {
		if !l.Allow("10.0.0.1") {
			t.Fatalf("request %d was refused inside the burst", i+1)
		}
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("a fourth request was allowed with a burst of three")
	}
}

// One caller's spending is not another's. A limit that pooled them would let
// any one address deny the service to everybody.
func TestCallersAreLimitedSeparately(t *testing.T) {
	l := api.NewLimit(1, 2)

	for i := 0; i < 2; i++ {
		l.Allow("10.0.0.1")
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("the first caller kept going")
	}
	if !l.Allow("10.0.0.2") {
		t.Fatal("a second caller was refused for the first one's spending")
	}
}

// The refusal is a 429 with a Retry-After: a client told to slow down and not
// told by how much retries immediately.
func TestARefusedCallerIsToldWhenToComeBack(t *testing.T) {
	h := api.NewLimited(ok(), api.NewLimit(1, 1), "Too many.")

	first := httptest.NewRecorder()
	h.ServeHTTP(first, request("10.0.0.1"))
	second := httptest.NewRecorder()
	h.ServeHTTP(second, request("10.0.0.1"))

	if first.Code != http.StatusOK {
		t.Fatalf("the first request was %d", first.Code)
	}
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("the second request was %d", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
}

/*
The one that matters.

Reading X-Forwarded-For without a proxy in front means the limit is keyed by a
value the caller picks — so a script that changes the header every request has
a fresh allowance every request, and the limit reads as working while doing
nothing.
*/
func TestAForwardedAddressIsIgnoredUnlessSomethingSetsIt(t *testing.T) {
	h := api.NewLimited(ok(), api.NewLimit(1, 1), "Too many.")

	for i, forged := range []string{"1.2.3.4", "5.6.7.8", "9.10.11.12"} {
		rec := httptest.NewRecorder()
		req := request("10.0.0.1")
		req.Header.Set("X-Forwarded-For", forged)
		h.ServeHTTP(rec, req)

		// The first is inside the burst; every later one must be refused
		// despite claiming to come from somewhere new.
		if i > 0 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("a forged X-Forwarded-For bought request %d a fresh allowance", i+1)
		}
	}
}

// And is honoured where something does set it, because otherwise every request
// through a load balancer shares one allowance and the limit is a global one.
func TestAForwardedAddressIsUsedBehindAProxy(t *testing.T) {
	h := api.NewLimited(ok(), api.NewLimit(1, 1), "Too many.").BehindProxy(true)

	for _, client := range []string{"1.2.3.4", "5.6.7.8"} {
		rec := httptest.NewRecorder()
		req := request("10.0.0.1") // the proxy, the same every time
		req.Header.Set("X-Forwarded-For", client+", 10.0.0.1")
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s was refused for another client's spending", client)
		}
	}
}

func request(from string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	r.RemoteAddr = from + ":54321"
	return r
}

/*
The finding the load harness made on its first run.

Eighty of a hundred renders were refused at a concurrency of one, because the
limit was keyed by address and everybody behind one NAT — or one load balancer
without CRONOS_BEHIND_PROXY — shared a single allowance. That is a limit that
throttles a real team and gets reported as "the reports are broken sometimes".
*/
func TestRendersAreLimitedPerReaderNotPerOffice(t *testing.T) {
	h := api.NewLimited(ok(), api.NewLimit(1, 1), "Too many.").By(api.ByBearer)

	// Two readers, one address, as an office is.
	for _, token := range []string{"reader-one", "reader-two"} {
		rec := httptest.NewRecorder()
		req := request("10.0.0.1")
		req.Header.Set("Authorization", "Bearer "+token)
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s was refused for a colleague's spending", token)
		}
	}

	// And one reader looping is still stopped.
	rec := httptest.NewRecorder()
	req := request("10.0.0.1")
	req.Header.Set("Authorization", "Bearer reader-one")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("one reader's second request was %d", rec.Code)
	}
}

// With no token there is nothing to key on but the address, and that is the
// right answer rather than an unlimited one.
func TestWithoutATokenTheAddressIsStillTheKey(t *testing.T) {
	h := api.NewLimited(ok(), api.NewLimit(1, 1), "Too many.").By(api.ByBearer)

	first := httptest.NewRecorder()
	h.ServeHTTP(first, request("10.0.0.1"))
	second := httptest.NewRecorder()
	h.ServeHTTP(second, request("10.0.0.1"))

	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("an anonymous caller was not limited: %d", second.Code)
	}
}

// The key is a hash. A map of live credentials held in memory for minutes is
// a credential store nobody meant to build.
func TestTheKeyIsNotTheToken(t *testing.T) {
	req := request("10.0.0.1")
	req.Header.Set("Authorization", "Bearer a-real-looking-token")

	if key := api.ByBearer(req); strings.Contains(key, "a-real-looking-token") {
		t.Fatalf("the key holds the token: %q", key)
	}
}
