package identity

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// MinPassword is the shortest password accepted.
//
// Twelve, and no composition rules. "One capital, one digit, one symbol"
// produces Password1! across every account in the building; length is the
// property that actually costs an attacker something.
const MinPassword = 12

// MaxPassword bounds what will be hashed, so a megabyte of input cannot be
// used to make the server do a megabyte of work per login attempt.
const MaxPassword = 1024

// Hash prepares a password for storage.
//
// SHA-256 first, then base64, then bcrypt. Not decoration: bcrypt silently
// ignores everything past 72 bytes, so two different passphrases sharing a
// 72-byte prefix would authenticate each other — and a passphrase is exactly
// the kind of password that runs long. Pre-hashing folds the whole input in.
// Base64 because the digest contains NUL bytes and bcrypt stops at the first
// one, which would throw away most of the entropy it was given.
func Hash(password string) (string, error) {
	if err := check(password); err != nil {
		return "", err
	}
	sum, err := bcrypt.GenerateFromPassword(prepare(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(sum), nil
}

// Verify reports whether the password matches the stored hash.
//
// It always does the work, even for an input that could not possibly be right.
// Returning early on a short password would make a wrong-length guess answer
// faster than a wrong-value one, which is a way to learn something about a
// password without ever guessing it.
func Verify(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), prepare(password)); err != nil {
		return ErrBadCredentials
	}
	return nil
}

func prepare(password string) []byte {
	if len(password) > MaxPassword {
		password = password[:MaxPassword]
	}
	sum := sha256.Sum256([]byte(password))
	return []byte(base64.RawStdEncoding.EncodeToString(sum[:]))
}

func check(password string) error {
	// Counted in runes. A twelve-character passphrase in a language that does
	// not use the Latin alphabet is not a short password.
	if utf8.RuneCountInString(strings.TrimSpace(password)) < MinPassword {
		return fmt.Errorf("%w: at least %d characters", ErrWeakPassword, MinPassword)
	}
	return nil
}
