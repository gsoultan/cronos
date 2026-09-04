package token_test

import (
	"errors"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Rotating the signing key without ending every session at once.

Before this, the key was the only one a build would verify against, so
replacing it invalidated every token in every host application at the instant
it took effect — an outage in somebody else's product, caused by our
housekeeping. The predictable result was that nobody rotated.

A retired key is accepted and never minted with. The rotation is: put the new
key in CRONOS_SIGNING_KEY, move the old one to CRONOS_SIGNING_KEY_PREVIOUS,
wait out token.MaxLifetime, drop it.
*/

var (
	oldKey = []byte("old-key-0123456789abcdef01234567")
	newKey = []byte("new-key-0123456789abcdef01234567")
)

func claims() token.Claims {
	return token.Claims{
		Audience: token.Embed, Org: "acme", Project: "finance",
		Subject: "c-1", Role: "viewer",
	}
}

func mintWith(t *testing.T, key []byte) string {
	t.Helper()

	s, err := token.NewSigner(key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestATokenFromTheOldKeyStillVerifiesDuringARotation(t *testing.T) {
	issued := mintWith(t, oldKey)

	rotated, err := token.NewSigner(newKey)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err = rotated.Accepting(oldKey)
	if err != nil {
		t.Fatal(err)
	}

	got, err := rotated.Verify(issued, token.Embed)
	if err != nil {
		t.Fatalf("a token signed by the retired key was refused mid-rotation: %v", err)
	}
	if got.Subject != "c-1" {
		t.Fatalf("verified into the wrong claims: %+v", got)
	}
}

// And once the old key is dropped, it stops working. A rotation that never
// finishes is two live keys for ever, which is not a rotation.
func TestATokenFromADroppedKeyIsRefused(t *testing.T) {
	issued := mintWith(t, oldKey)

	after, err := token.NewSigner(newKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := after.Verify(issued, token.Embed); !errors.Is(err, token.ErrInvalid) {
		t.Fatalf("a token signed by a dropped key was accepted, or failed oddly: %v", err)
	}
}

// New tokens are signed with the current key, never a retired one — otherwise
// the rotation extends itself every time something is minted.
func TestMintingUsesTheCurrentKeyOnly(t *testing.T) {
	rotated, err := token.NewSigner(newKey)
	if err != nil {
		t.Fatal(err)
	}
	if rotated, err = rotated.Accepting(oldKey); err != nil {
		t.Fatal(err)
	}

	raw, err := rotated.Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// A build that only knows the old key must not accept it.
	only, err := token.NewSigner(oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := only.Verify(raw, token.Embed); err == nil {
		t.Fatal("a freshly minted token verified against the retired key: " +
			"Mint is still signing with it")
	}
}

// A retired key short enough to brute-force is refused rather than accepted on
// the way out. It was a signing key once; that is not a reason to keep it.
func TestAWeakRetiredKeyIsRefused(t *testing.T) {
	s, err := token.NewSigner(newKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Accepting([]byte("short")); !errors.Is(err, token.ErrWeakKey) {
		t.Fatalf("a five-byte retired key was accepted: %v", err)
	}
}

// Expiry is still checked against a retired key. Accepting the old signature
// must not also accept an old token that has run out.
func TestAnExpiredTokenFromTheOldKeyIsStillExpired(t *testing.T) {
	past, err := token.NewSigner(oldKey)
	if err != nil {
		t.Fatal(err)
	}
	long := time.Now().Add(-2 * time.Hour)
	issued, err := past.WithClock(func() time.Time { return long }).Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	rotated, err := token.NewSigner(newKey)
	if err != nil {
		t.Fatal(err)
	}
	if rotated, err = rotated.Accepting(oldKey); err != nil {
		t.Fatal(err)
	}
	if _, err := rotated.Verify(issued, token.Embed); !errors.Is(err, token.ErrInvalid) {
		t.Fatalf("an expired token was accepted because its key was retired: %v", err)
	}
}
