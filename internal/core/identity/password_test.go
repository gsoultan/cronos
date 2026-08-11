package identity

import (
	"errors"
	"strings"
	"testing"
)

func TestARoundTrip(t *testing.T) {
	hash, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "horse") {
		t.Fatal("the password is in the hash")
	}
	if err := Verify(hash, "correct horse battery staple"); err != nil {
		t.Errorf("the right password was refused: %v", err)
	}
	if err := Verify(hash, "correct horse battery stapler"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("a wrong password was accepted: %v", err)
	}
}

// bcrypt ignores everything past 72 bytes, so two passphrases sharing a
// 72-byte prefix would authenticate each other. A passphrase is exactly the
// kind of password that runs long.
func TestLongPasswordsAreNotTruncated(t *testing.T) {
	prefix := strings.Repeat("a", 72)
	hash, err := Hash(prefix + "-one")
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(hash, prefix+"-two"); !errors.Is(err, ErrBadCredentials) {
		t.Fatal("two passwords sharing a 72-byte prefix authenticated each other")
	}
	if err := Verify(hash, prefix+"-one"); err != nil {
		t.Errorf("the right long password was refused: %v", err)
	}
}

// The same salt would make two identical passwords produce the same hash, and
// a leaked table would then say which accounts share one.
func TestTheSamePasswordHashesDifferentlyEachTime(t *testing.T) {
	a, _ := Hash("correct horse battery staple")
	b, _ := Hash("correct horse battery staple")
	if a == b {
		t.Error("hashes are not salted")
	}
}

func TestShortPasswordsAreRefusedWhereTheyAreSet(t *testing.T) {
	for _, p := range []string{"", "short", strings.Repeat("a", MinPassword-1), "           "} {
		if _, err := Hash(p); !errors.Is(err, ErrWeakPassword) {
			t.Errorf("accepted %q", p)
		}
	}
	// Counted in runes, so twelve characters is twelve characters whatever
	// alphabet they are written in. (This one is sixteen.)
	if _, err := Hash("パスワードは日本語でも大丈夫です"); err != nil {
		t.Errorf("a sixteen-rune password was refused: %v", err)
	}
}

// Returning early on an impossible input would make a wrong-length guess answer
// faster than a wrong-value one.
func TestVerifyDoesTheWorkForAnyInput(t *testing.T) {
	hash, _ := Hash("correct horse battery staple")
	if err := Verify(hash, "x"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("got %v", err)
	}
	// A megabyte of input must not become a megabyte of work.
	if err := Verify(hash, strings.Repeat("x", 5_000_000)); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("got %v", err)
	}
}

// A hash from somewhere else, or a corrupt column, must fail closed.
func TestAnUnusableHashRefuses(t *testing.T) {
	for _, h := range []string{"", "not-a-hash", "$2a$10$tooshort"} {
		if err := Verify(h, "correct horse battery staple"); !errors.Is(err, ErrBadCredentials) {
			t.Errorf("hash %q gave %v", h, err)
		}
	}
}
