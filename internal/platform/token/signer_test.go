package token

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/core/principal"
)

var at = func(s string) func() time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return func() time.Time { return t }
}

func signer(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner([]byte("a-key-that-is-long-enough-to-sign-with"))
	if err != nil {
		t.Fatal(err)
	}
	return s.WithClock(at("2026-08-11T12:00:00Z"))
}

func claims() Claims {
	return Claims{
		Org: "o1", Project: "p1", Subject: "acme-user-42",
		Report: "monthly-invoice-statement",
		Scope:  map[string]string{"customer_id": "c-9"},
	}
}

func TestARoundTrip(t *testing.T) {
	s := signer(t)
	tok, err := s.Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Verify(tok)
	if err != nil {
		t.Fatalf("a token we just minted does not verify: %v", err)
	}
	if got.Scope["customer_id"] != "c-9" || got.Report != "monthly-invoice-statement" {
		t.Errorf("claims did not survive: %+v", got)
	}
	if got.ExpiresAt-got.IssuedAt != 3600 {
		t.Errorf("lifetime = %ds, want 3600", got.ExpiresAt-got.IssuedAt)
	}
}

// The claims a token carries become the identity the query compiles against.
// An embed token can never be anything but a viewer.
func TestAnEmbedTokenIsAlwaysAViewer(t *testing.T) {
	pr := claims().Principal()
	if pr.ProjectRole != principal.ProjectViewer {
		t.Errorf("role = %q, want viewer", pr.ProjectRole)
	}
	if pr.CanEdit() || pr.CanAdminProject() || pr.CanAdminOrg() {
		t.Error("an end customer of our customer must not be able to edit anything")
	}
	if pr.Scope["customer_id"] != "c-9" {
		t.Error("the scope claim is the whole point and did not arrive")
	}
}

func TestForgeriesAreRefused(t *testing.T) {
	s := signer(t)
	valid, err := s.Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(valid, ".")

	// Rewrite the scope and re-encode, leaving the signature alone. This is
	// the attack the whole design exists to stop: an end user granting
	// themselves another customer's rows.
	widened := func() string {
		c := claims()
		c.Scope = map[string]string{"customer_id": "c-1"}
		c.ExpiresAt = time.Now().Add(time.Hour).Unix()
		b, _ := json.Marshal(c)
		return parts[0] + "." + base64.RawURLEncoding.EncodeToString(b) + "." + parts[2]
	}()

	cases := map[string]string{
		"a rewritten scope":        widened,
		"a stripped signature":     parts[0] + "." + parts[1] + ".",
		"no signature at all":      parts[0] + "." + parts[1],
		"a flipped signature byte": parts[0] + "." + parts[1] + "." + flip(parts[2]),
		"a different version":      "v2." + parts[1] + "." + parts[2],
		"someone else's token":     "v1.e30.e30",
		"nothing":                  "",
	}

	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Verify(tok); !errors.Is(err, ErrInvalid) {
				t.Errorf("accepted %s: %v", name, err)
			}
		})
	}
}

// A different key must not verify, or every deployment shares a trust root.
func TestAnotherKeyDoesNotVerify(t *testing.T) {
	tok, err := signer(t).Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewSigner([]byte("a-different-key-that-is-also-long-enough"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.WithClock(at("2026-08-11T12:00:00Z")).Verify(tok); !errors.Is(err, ErrInvalid) {
		t.Errorf("a token signed elsewhere verified: %v", err)
	}
}

func TestExpiry(t *testing.T) {
	s := signer(t)
	tok, err := s.Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.WithClock(at("2026-08-11T12:59:00Z")).Verify(tok); err != nil {
		t.Errorf("still valid at 59 minutes: %v", err)
	}
	if _, err := s.WithClock(at("2026-08-11T13:05:00Z")).Verify(tok); !errors.Is(err, ErrInvalid) {
		t.Error("an expired token verified")
	}
	// Clocks disagree; a token must not die the instant one of them ticks.
	if _, err := s.WithClock(at("2026-08-11T13:00:20Z")).Verify(tok); err != nil {
		t.Errorf("20 seconds of skew should be tolerated: %v", err)
	}
}

// Treating a missing exp as "never expires" is how one leaked token stays
// useful for years.
func TestATokenWithNoExpiryIsRefused(t *testing.T) {
	s := signer(t)
	c := claims()
	b, _ := json.Marshal(c) // IssuedAt and ExpiresAt both zero
	body := "v1." + base64.RawURLEncoding.EncodeToString(b)
	tok := body + "." + base64.RawURLEncoding.EncodeToString(s.sign(body))

	if _, err := s.Verify(tok); !errors.Is(err, ErrInvalid) {
		t.Error("a correctly signed token with no expiry was accepted")
	}
}

// One error for every failure. Telling a caller which half of their forgery
// worked turns verification into an oracle.
func TestFailuresAreIndistinguishable(t *testing.T) {
	s := signer(t)
	var seen []string
	for _, tok := range []string{"", "v1.aaa.bbb", "v9.aaa.bbb", "garbage"} {
		if _, err := s.Verify(tok); err != nil {
			seen = append(seen, errors.Unwrap(err).Error())
		}
	}
	for _, e := range seen {
		if e != ErrInvalid.Error() {
			t.Errorf("failures should share one sentinel, got %q", e)
		}
	}
}

func TestMintRefuses(t *testing.T) {
	s := signer(t)
	cases := map[string]struct {
		c        Claims
		lifetime time.Duration
	}{
		// A permanent credential in a browser.
		"a lifetime beyond the maximum": {claims(), 48 * time.Hour},
		"a lifetime of nothing":         {claims(), 0},
		// Would authenticate into whatever the caller asked for.
		"no organization": {Claims{Project: "p1"}, time.Hour},
		"no project":      {Claims{Org: "o1"}, time.Hour},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Mint(c.c, c.lifetime); err == nil {
				t.Errorf("minted %s", name)
			}
		})
	}
}

// HMAC-SHA256 with a short key is HMAC-SHA256 with a guessable key, and this
// is a configuration mistake an operator should meet at startup.
func TestAShortKeyIsRefusedAtStartup(t *testing.T) {
	if _, err := NewSigner([]byte("short")); !errors.Is(err, ErrWeakKey) {
		t.Errorf("got %v, want ErrWeakKey", err)
	}
}

func flip(s string) string {
	if s == "" {
		return "x"
	}
	b := []byte(s)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	return string(b)
}
