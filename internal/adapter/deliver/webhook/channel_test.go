package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/app/burst"
)

var secret = []byte("a-webhook-secret-of-at-least-32-bytes")

type received struct {
	header http.Header
	body   []byte
}

// receiver answers with status and keeps what it was sent.
func receiver(t *testing.T, status int) (*httptest.Server, *[]received) {
	t.Helper()
	var mu sync.Mutex
	var got []received
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, received{r.Header.Clone(), body})
		mu.Unlock()
		w.WriteHeader(status)
		io.WriteString(w, "noted")
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func channel(t *testing.T, hosts ...string) *Channel {
	t.Helper()
	c, err := New(Config{Secret: secret, Hosts: hosts,
		Now: func() time.Time { return time.Unix(1791700000, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func delivery(to string) burst.Delivery {
	return burst.Delivery{To: to, Filename: "statement-c-1.pdf",
		Subject: "Your October statement", Document: []byte("%PDF-1.7 c-1")}
}

// What a receiver does with the signature, done here: recompute it from the
// timestamp and the body, and compare.
func TestTheDocumentArrivesSignedAndNamed(t *testing.T) {
	srv, got := receiver(t, http.StatusOK)
	c := channel(t, "127.0.0.1")
	if err := c.Deliver(context.Background(), delivery(srv.URL+"/inbound")); err != nil {
		t.Fatal(err)
	}
	r := (*got)[0]
	if string(r.body) != "%PDF-1.7 c-1" || r.header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("got %q as %s", r.body, r.header.Get("Content-Type"))
	}
	if !strings.Contains(r.header.Get("Content-Disposition"), `filename=statement-c-1.pdf`) {
		t.Errorf("disposition %q", r.header.Get("Content-Disposition"))
	}
	ts, sig, _ := strings.Cut(strings.TrimPrefix(r.header.Get("Cronos-Signature"), "t="), ",v1=")
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(ts + "."))
	mac.Write(r.body)
	if ts != "1791700000" || !hmac.Equal([]byte(sig), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		t.Errorf("the signature does not verify: %s", r.header.Get("Cronos-Signature"))
	}
}

// A retry is the same delivery, so a receiver can keep one copy.
func TestEveryAttemptCarriesOneIdempotencyKey(t *testing.T) {
	srv, got := receiver(t, http.StatusServiceUnavailable)
	c := channel(t, "127.0.0.1")
	for range 2 {
		c.Deliver(context.Background(), delivery(srv.URL))
	}
	other := delivery(srv.URL)
	other.Document = []byte("%PDF-1.7 c-2")
	c.Deliver(context.Background(), other)

	k := func(i int) string { return (*got)[i].header.Get("Idempotency-Key") }
	if k(0) == "" || k(0) != k(1) || k(0) == k(2) {
		t.Fatalf("keys %q %q %q", k(0), k(1), k(2))
	}
}

func TestWhatIsRetriedAndWhatIsNot(t *testing.T) {
	for status, retried := range map[int]bool{
		500: true, 503: true, 408: true, 429: true,
		400: false, 401: false, 404: false, 410: false, 302: false,
	} {
		srv, _ := receiver(t, status)
		err := channel(t, "127.0.0.1").Deliver(context.Background(), delivery(srv.URL))
		var p burst.Permanent
		if err == nil || errors.As(err, &p) == retried {
			t.Errorf("%d: %v, want retried=%v", status, err, retried)
		}
	}
}

// The URL comes from a schedule and often from a customer's row. It goes
// nowhere the deployment did not list, and never in the clear past loopback.
func TestADeliveryGoesOnlyWhereTheDeploymentAllows(t *testing.T) {
	srv, got := receiver(t, http.StatusOK)
	u, _ := url.Parse(srv.URL)
	c := channel(t, "127.0.0.1", "hooks.example.com")
	for _, to := range []string{
		"http://localhost:" + u.Port(),       // loopback, but not listed
		"http://hooks.example.com/in",        // listed, but plain http
		"https://evil.example.net/in",        // not listed
		"https://user:pw@hooks.example.com/", // credentials in the URL
		"ftp://hooks.example.com/in",
		"not a url",
	} {
		err := c.Deliver(context.Background(), delivery(to))
		var p burst.Permanent
		if !errors.As(err, &p) {
			t.Errorf("%s: %v", to, err)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("%d requests reached a receiver", len(*got))
	}
}

// Following a redirect is going somewhere the list did not name.
func TestARedirectIsNotFollowed(t *testing.T) {
	target, got := receiver(t, http.StatusOK)
	hop := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusTemporaryRedirect))
	t.Cleanup(hop.Close)
	err := channel(t, "127.0.0.1").Deliver(context.Background(), delivery(hop.URL))
	if err == nil || len(*got) != 0 {
		t.Fatalf("followed a redirect: %v, %d requests", err, len(*got))
	}
}

func TestAChannelNeedsASecretAndAHost(t *testing.T) {
	if _, err := New(Config{Secret: []byte("short"), Hosts: []string{"h"}}); err == nil {
		t.Error("a guessable secret")
	}
	if _, err := New(Config{Secret: secret}); err == nil {
		t.Error("a channel that may post anywhere, or nowhere")
	}
}
