package token

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

var (
	embedKey  = []byte("the-key-a-host-application-is-given")
	portalKey = []byte("the-key-that-never-leaves-cronos-xx")
	noon      = at("2026-08-11T12:00:00Z")
)

// signedBy is what a host's backend does: it holds a key, so it signs whatever
// claims it likes, in code cronos never sees. Mint's own rules do not apply.
func signedBy(t *testing.T, key []byte, c Claims) string {
	t.Helper()
	payload, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	body := version + "." + enc.EncodeToString(payload)
	return body + "." + enc.EncodeToString(mac(key, body))
}

func keyed(t *testing.T, key []byte) *Signer {
	t.Helper()
	s, err := NewSigner(key)
	if err != nil {
		t.Fatal(err)
	}
	return s.WithClock(noon)
}

func hostClaims(audience string) Claims {
	return Claims{
		Audience: audience, Org: "acme", Project: "finance", Subject: "host-backend",
		Role: "admin", OrgRole: "owner", Platform: true,
		IssuedAt: noon().Unix(), ExpiresAt: noon().Add(time.Hour).Unix(),
	}
}

/*
The hole this closes. A host application's backend holds the embed key, and so
chooses every claim it signs — the audience included. While one key signed both
kinds of token, `aud: portal` with a platform claim, signed by a host, was a
session that administered the deployment.
*/
func TestTheKeyAHostHoldsCannotSignAPortalSession(t *testing.T) {
	forged := signedBy(t, embedKey, hostClaims(Portal))

	if _, err := keyed(t, portalKey).Verify(forged, Portal); !errors.Is(err, ErrInvalid) {
		t.Fatalf("the portal accepted a session signed with the embed key: %v", err)
	}
	// And the signer made from the embed key refuses it too, so wiring that
	// one into the portal by mistake accepts nothing rather than everything.
	if _, err := keyed(t, embedKey).For(Embed).Verify(forged, Portal); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a signer confined to embed tokens verified a portal one: %v", err)
	}
}

func TestTheEmbedKeyStillSignsWhatItIsFor(t *testing.T) {
	s := keyed(t, embedKey).For(Embed)
	got, err := s.Verify(signedBy(t, embedKey, hostClaims(Embed)), Embed)
	if err != nil {
		t.Fatalf("a host's own embed token does not verify: %v", err)
	}
	// Whatever it claimed on the way in.
	if pr := got.Principal(); pr.Platform || pr.Member || pr.OrgRole != "" {
		t.Fatalf("an embed token carried more than a viewer: %+v", pr)
	}
}

func TestAConfinedSignerMintsOnlyItsAudience(t *testing.T) {
	s := keyed(t, embedKey).For(Embed)
	if _, err := s.Mint(Claims{Audience: Portal, Org: "acme", Project: "finance"}, time.Hour); err == nil {
		t.Fatal("the embed key minted a portal token")
	}
	if _, err := s.Mint(Claims{Audience: Embed, Org: "acme", Project: "finance"}, time.Hour); err != nil {
		t.Fatalf("and would not mint an embed one: %v", err)
	}
}

// Confinement survives the copies a signer is passed around as. One that
// dropped it in Accepting would be unconfined exactly during a rotation.
func TestConfinementSurvivesRotationAndAClock(t *testing.T) {
	s, err := keyed(t, embedKey).For(Embed).Accepting(portalKey)
	if err != nil {
		t.Fatal(err)
	}
	s = s.WithClock(noon)
	if _, err := s.Verify(signedBy(t, portalKey, hostClaims(Portal)), Portal); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a rotated embed signer verified a portal token: %v", err)
	}
}

/*
Mint refuses a lifetime over the maximum, and that was the only place it was
held — which covers every token cronos signs and none a host signs. A token
signed for ten years, with a key the host holds, verified.
*/
func TestATokenAHostSignedForYearsIsRefused(t *testing.T) {
	s := keyed(t, embedKey).For(Embed)
	c := hostClaims(Embed)

	c.ExpiresAt = noon().Add(10 * 365 * 24 * time.Hour).Unix()
	if _, err := s.Verify(signedBy(t, embedKey, c), Embed); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a ten-year token verified: %v", err)
	}

	// Leaving out when it was issued does not hide how long it lives.
	c.IssuedAt = 0
	if _, err := s.Verify(signedBy(t, embedKey, c), Embed); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a token with no issue time verified: %v", err)
	}

	// The longest Mint would issue still verifies.
	c = hostClaims(Embed)
	c.ExpiresAt = noon().Add(MaxLifetime).Unix()
	if _, err := s.Verify(signedBy(t, embedKey, c), Embed); err != nil {
		t.Fatalf("a token at the maximum lifetime was refused: %v", err)
	}
}
