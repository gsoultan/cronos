package secret_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/platform/secret"
)

var (
	keyA = []byte("a-secrets-key-that-is-long-enough-to-seal")
	keyB = []byte("another-secrets-key-long-enough-to-seal!!")
)

func sealer(t *testing.T, key []byte, retired ...[]byte) *secret.Sealer {
	t.Helper()
	s, err := secret.NewSealer(key, retired...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestASealedValueOpensWhereItWasSealed(t *testing.T) {
	s := sealer(t, keyA)
	sealed := s.Seal("pk.eyJ1Ijoi", "acme", "finance", "mapbox-token")

	if bytes.Contains(sealed, []byte("pk.eyJ1Ijoi")) {
		t.Fatal("the value is in the sealed bytes as it was typed")
	}
	got, err := s.Open(sealed, "acme", "finance", "mapbox-token")
	if err != nil || got != "pk.eyJ1Ijoi" {
		t.Fatalf("Open = %q, %v", got, err)
	}
}

// Somebody who can write to the table and cannot read the key must not be able
// to move one project's warehouse password into a project they edit.
func TestASealedValueDoesNotOpenAnywhereElse(t *testing.T) {
	s := sealer(t, keyA)
	sealed := s.Seal("hunter2", "acme", "finance", "warehouse-password")

	for _, where := range [][]string{
		{"acme", "sales", "warehouse-password"},
		{"globex", "finance", "warehouse-password"},
		{"acme", "finance", "mapbox-token"},
		// The parts are length-prefixed, so shifting a character from one to
		// the next is somewhere else too.
		{"acm", "efinance", "warehouse-password"},
	} {
		if _, err := s.Open(sealed, where...); !errors.Is(err, secret.ErrSealed) {
			t.Errorf("opened under %v: %v", where, err)
		}
	}
}

func TestADamagedValueDoesNotOpen(t *testing.T) {
	s := sealer(t, keyA)
	sealed := s.Seal("hunter2", "acme", "finance", "pw")

	for i := range sealed {
		damaged := bytes.Clone(sealed)
		damaged[i] ^= 0x01
		if _, err := s.Open(damaged, "acme", "finance", "pw"); !errors.Is(err, secret.ErrSealed) {
			t.Fatalf("a flipped bit at %d still opened: %v", i, err)
		}
	}
	if _, err := s.Open(sealed[:5], "acme", "finance", "pw"); !errors.Is(err, secret.ErrSealed) {
		t.Errorf("a truncated value opened: %v", err)
	}
}

// The same value sealed twice is two different sets of bytes, so the table
// does not say which projects share a password.
func TestSealingTheSameValueTwiceDiffers(t *testing.T) {
	s := sealer(t, keyA)
	if bytes.Equal(s.Seal("x", "a", "b", "c"), s.Seal("x", "a", "b", "c")) {
		t.Fatal("two seals of one value are identical")
	}
}

func TestAnotherKeyCannotOpenIt(t *testing.T) {
	sealed := sealer(t, keyA).Seal("hunter2", "acme", "finance", "pw")
	if _, err := sealer(t, keyB).Open(sealed, "acme", "finance", "pw"); !errors.Is(err, secret.ErrSealed) {
		t.Fatalf("a different key opened it: %v", err)
	}
}

// Rotation: the new key in, the old one retired. What the old one sealed still
// opens, is reported stale, and once sealed again is not.
func TestARetiredKeyStillOpensWhatItSealed(t *testing.T) {
	old := sealer(t, keyA).Seal("hunter2", "acme", "finance", "pw")
	rotated := sealer(t, keyB, keyA)

	got, err := rotated.Open(old, "acme", "finance", "pw")
	if err != nil || got != "hunter2" {
		t.Fatalf("Open after rotation = %q, %v", got, err)
	}
	if !rotated.Stale(old) {
		t.Error("a value sealed by the retired key is not stale")
	}
	again := rotated.Seal(got, "acme", "finance", "pw")
	if rotated.Stale(again) {
		t.Error("a value sealed by the current key is stale")
	}
	// And once the retired key is dropped, the resealed value is what survives.
	if _, err := sealer(t, keyB).Open(again, "acme", "finance", "pw"); err != nil {
		t.Errorf("the resealed value needs the retired key: %v", err)
	}
}

func TestAShortKeyIsRefused(t *testing.T) {
	_, err := secret.NewSealer([]byte("short"))
	if !errors.Is(err, secret.ErrWeakKey) {
		t.Fatalf("a five-byte key was accepted: %v", err)
	}
	_, err = secret.NewSealer(keyA, []byte("short"))
	if !errors.Is(err, secret.ErrWeakKey) || !strings.Contains(err.Error(), "previous") {
		t.Fatalf("a short retired key was accepted or not named: %v", err)
	}
}
