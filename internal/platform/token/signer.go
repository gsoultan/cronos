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

// Audiences a token may be minted for.
const (
	// Embed is an end customer of our customer: one report, row-scoped, viewer
	// for ever.
	Embed = "embed"
	// Portal is somebody who authors reports. Carries a project role.
	Portal = "portal"
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

// Signer mints and verifies tokens with one key, and verifies with any number
// of keys it no longer mints with.
type Signer struct {
	key []byte
	/*
	   retired are accepted on Verify and never minted with.

	   Without them a rotation is an outage. Every embed token in every host
	   application was signed with the old key, and the moment the new one is
	   in place all of them fail — for as long as it takes each of our
	   customers to notice, and then to mint again. That is an outage this
	   product causes in somebody else's application, so in practice the key
	   never rotates, which is the worse outcome.

	   With them, the rotation is: add the new key, keep the old one here,
	   wait out MaxLifetime, drop it. Nothing signed by a retired key can be
	   longer-lived than that, because Mint would not have issued it.
	*/
	retired [][]byte
	now     func() time.Time
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
	return &Signer{key: s.key, retired: s.retired, now: now}
}

/*
Accepting returns a copy that also verifies tokens signed by keys.

For a rotation, and for nothing else — these keys are never minted with, so a
token this deployment issues is always signed by the current one. A key too
short to have been a signing key is refused here too: it was one once, and
accepting a weak key on the way out is accepting it.
*/
func (s *Signer) Accepting(keys ...[]byte) (*Signer, error) {
	retired := make([][]byte, 0, len(s.retired)+len(keys))
	retired = append(retired, s.retired...)
	for _, k := range keys {
		if len(k) < MinKeyBytes {
			return nil, fmt.Errorf("%w: retired key of %d bytes, need at least %d",
				ErrWeakKey, len(k), MinKeyBytes)
		}
		retired = append(retired, k)
	}
	return &Signer{key: s.key, retired: retired, now: s.now}, nil
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
	case c.Audience != Embed && c.Audience != Portal:
		return "", fmt.Errorf("token: audience must be %q or %q", Embed, Portal)
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

// Verify checks a token minted for the given audience.
//
// The audience is a parameter rather than a field the caller reads afterwards,
// because a check somebody has to remember is a check somebody forgets — and
// the thing forgotten would be an embed token opening the management API.
//
// The signature is checked before anything in the payload is read. Parsing
// attacker-controlled JSON and then deciding whether to trust it gets the
// order backwards: every parser bug becomes reachable by anyone.
func (s *Signer) Verify(raw, audience string) (Claims, error) {
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
	if !s.signed(prefix+"."+encoded, got) {
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
	if c.Audience != audience {
		return Claims{}, fmt.Errorf("%w: audience", ErrInvalid)
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

func (s *Signer) sign(body string) []byte { return mac(s.key, body) }

/*
signed says whether any key this deployment accepts produced got.

The current key first, so the ordinary token costs one HMAC. Every key is
compared in constant time; the loop's own early exit leaks which key matched,
which is a fact about this deployment's rotation and not about the secret.
*/
func (s *Signer) signed(body string, got []byte) bool {
	if subtle.ConstantTimeCompare(got, mac(s.key, body)) == 1 {
		return true
	}
	for _, k := range s.retired {
		if subtle.ConstantTimeCompare(got, mac(k, body)) == 1 {
			return true
		}
	}
	return false
}

// mac is the signature over body under key.
func mac(key []byte, body string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(body))
	return m.Sum(nil)
}

// Principal turns verified claims into the identity a request runs as.
//
// An embed token is always a viewer, whatever it claims. It is issued to an
// end customer of our customer: they read one report and never edit anything,
// so there is no claim that could raise it and nothing to forge. A portal
// token carries a role, because an author genuinely has one.
func (c Claims) Principal() principal.Principal {
	role := principal.ProjectViewer
	if c.Audience == Portal {
		switch principal.Role(c.Role) {
		case principal.ProjectAdmin, principal.ProjectEditor:
			role = principal.Role(c.Role)
		}
	}
	return principal.Principal{
		Subject:     c.Subject,
		OrgID:       c.Org,
		ProjectID:   c.Project,
		ProjectRole: role,
		Scope:       c.Scope,
		// Only a portal token. An embed token belongs to an end customer, and
		// the exemption is the one thing it must never be able to claim.
		Member: c.Audience == Portal,
		// The same rule, for the same reason. An embed token is minted by a
		// host application for one of its customers; a claim it could raise to
		// deployment administrator would make every one of them one.
		Platform: c.Audience == Portal && c.Platform,
		// The same rule again. An embed token that could claim this would be
		// claiming *less*, so it is harmless — and it is set the same way as
		// its neighbours so nobody reading later has to work out why one of
		// three is different.
		Enrol: c.Audience == Portal && c.Enrol,
	}
}
