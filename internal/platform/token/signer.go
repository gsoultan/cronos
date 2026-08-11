package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/core/principal"
)

const (
	// version prefixes every token and is covered by the signature. Without
	// that, a future v2 with different rules could be downgraded to v1 by
	// rewriting three characters.
	version = "v1"
	// MinKeyBytes is the shortest signing key accepted. HMAC-SHA256 with a
	// short key is HMAC-SHA256 with a guessable key.
	MinKeyBytes = 32
	// MaxLifetime bounds what Mint will issue. An embed token lives in a
	// browser, and a long-lived one is a permanent credential sitting in
	// somebody's devtools.
	MaxLifetime = 24 * time.Hour
	// skew allows for clocks that disagree. Small: the cost of a generous
	// window is a token that outlives its revocation.
	skew = 30 * time.Second
)

var enc = base64.RawURLEncoding

// Signer mints and verifies tokens with one key.
type Signer struct {
	key []byte
	now func() time.Time
}

// NewSigner returns a Signer over key, or an error if the key is too weak to
// be worth signing with.
func NewSigner(key []byte) (*Signer, error) {
	if len(key) < MinKeyBytes {
		return nil, fmt.Errorf("%w: %d bytes, need at least %d",
			ErrWeakKey, len(key), MinKeyBytes)
	}
	return &Signer{key: key, now: time.Now}, nil
}

// WithClock returns a copy reading time from now. For tests, and for a
// scheduled mint that must date a token from the run rather than from whenever
// a worker got to it.
func (s *Signer) WithClock(now func() time.Time) *Signer {
	return &Signer{key: s.key, now: now}
}

// Mint issues a token valid for lifetime.
func (s *Signer) Mint(c Claims, lifetime time.Duration) (string, error) {
	switch {
	case lifetime <= 0:
		return "", fmt.Errorf("token: lifetime must be positive")
	case lifetime > MaxLifetime:
		return "", fmt.Errorf("token: lifetime %s exceeds the %s maximum", lifetime, MaxLifetime)
	case c.Org == "" || c.Project == "":
		// A token with no tenancy would authenticate into whatever the caller
		// asked for. Refusing at mint is the cheapest place to catch it.
		return "", fmt.Errorf("token: org and project are required")
	}

	issued := s.now()
	c.IssuedAt = issued.Unix()
	c.ExpiresAt = issued.Add(lifetime).Unix()

	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	body := version + "." + enc.EncodeToString(payload)
	return body + "." + enc.EncodeToString(s.sign(body)), nil
}

// Verify checks a token and returns what it asserts.
//
// The signature is checked before anything in the payload is read. Parsing
// attacker-controlled JSON and then deciding whether to trust it gets the
// order backwards — every parser bug becomes reachable by anyone.
func (s *Signer) Verify(raw string) (Claims, error) {
	prefix, rest, ok := strings.Cut(raw, ".")
	if !ok || prefix != version {
		return Claims{}, fmt.Errorf("%w: not a %s token", ErrInvalid, version)
	}
	encoded, mac, ok := strings.Cut(rest, ".")
	if !ok {
		return Claims{}, fmt.Errorf("%w: malformed", ErrInvalid)
	}

	got, err := enc.DecodeString(mac)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: malformed signature", ErrInvalid)
	}
	// Constant time: a byte-by-byte comparison leaks how much of a forged
	// signature was right, which is enough to construct the rest.
	if subtle.ConstantTimeCompare(got, s.sign(prefix+"."+encoded)) != 1 {
		return Claims{}, fmt.Errorf("%w: signature", ErrInvalid)
	}

	payload, err := enc.DecodeString(encoded)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: malformed payload", ErrInvalid)
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Claims{}, fmt.Errorf("%w: payload", ErrInvalid)
	}
	return c, s.check(c)
}

func (s *Signer) check(c Claims) error {
	now := s.now()
	switch {
	case c.ExpiresAt == 0:
		// A token with no expiry is a permanent credential. Treating a missing
		// claim as "never expires" is how one leaked token stays useful for
		// years.
		return fmt.Errorf("%w: no expiry", ErrInvalid)
	case now.After(time.Unix(c.ExpiresAt, 0).Add(skew)):
		return fmt.Errorf("%w: expired", ErrInvalid)
	case c.IssuedAt != 0 && now.Add(skew).Before(time.Unix(c.IssuedAt, 0)):
		return fmt.Errorf("%w: issued in the future", ErrInvalid)
	case c.Org == "" || c.Project == "":
		return fmt.Errorf("%w: no tenancy", ErrInvalid)
	}
	return nil
}

func (s *Signer) sign(body string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(body))
	return m.Sum(nil)
}

// Principal turns verified claims into the identity a request runs as.
//
// The role is fixed at viewer and is not taken from the token. An embed token
// is issued to an end customer of our customer: they read one report and they
// never edit anything, so there is no claim that could raise it and nothing to
// forge.
func (c Claims) Principal() principal.Principal {
	return principal.Principal{
		Subject:     c.Subject,
		OrgID:       c.Org,
		ProjectID:   c.Project,
		ProjectRole: principal.ProjectViewer,
		Scope:       c.Scope,
	}
}
