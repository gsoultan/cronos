package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

/*
Recovery codes: the way back in when the phone is gone.

Without them a second factor is a way to lose an account. Phones are dropped,
wiped and replaced, and an authenticator app does not come back with the
contacts — so the realistic alternative to recovery codes is an administrator
turning somebody's second factor off over chat, which is a social-engineering
path straight past the thing that was just enabled.

They are passwords in every respect that matters, so they are treated as such:
generated here, shown once, stored as hashes, and spent on use.
*/

// RecoveryCodes is how many are issued. Ten is enough that somebody who uses
// one and forgets to regenerate is not two mistakes from being locked out, and
// few enough that the list fits where people actually keep it.
const RecoveryCodes = 10

/*
NewRecoveryCodes mints a set and the hashes to store.

Returned in step, so a caller cannot store one set and show another — the bug
that turns "here are your recovery codes" into ten strings that open nothing,
and which nobody discovers until the day one is needed.
*/
func NewRecoveryCodes() (codes []string, hashes []string, err error) {
	codes = make([]string, 0, RecoveryCodes)
	hashes = make([]string, 0, RecoveryCodes)

	for range RecoveryCodes {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		codes = append(codes, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return codes, hashes, nil
}

/*
newRecoveryCode is ten characters, in two groups, from an unambiguous alphabet.

No 0/O, no 1/I/L. These are read off a screenshot or a piece of paper months
later, often by somebody who is already locked out and not at their best, and a
character somebody cannot tell apart is a code that does not work for a reason
nobody can see.

Base32's alphabet is not it — it contains O and I. This is Crockford's, which
was designed for exactly this and which excludes U as well, to avoid producing
words nobody wants printed on their recovery sheet.
*/
const recoveryAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newRecoveryCode() (string, error) {
	// 10 characters from a 32-symbol alphabet is 50 bits. Far beyond guessing,
	// and the rate limit on the sign-in route is what makes even that moot.
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	out := make([]byte, 0, 11)
	for i, v := range b {
		if i == 5 {
			// A hyphen in the middle, because a ten-character run is one people
			// lose their place in when reading it aloud or typing it.
			out = append(out, '-')
		}
		out = append(out, recoveryAlphabet[int(v)%len(recoveryAlphabet)])
	}
	return string(out), nil
}

/*
HashRecoveryCode is how one is stored and looked up.

SHA-256, not bcrypt, for the same reason an invitation secret is: fifty bits
from a CSPRNG has nothing to brute force, and the cost would buy a slower
sign-in and nothing else. What it does buy is what it buys for passwords —
somebody who reads the table cannot use what they read.

Normalised first. People type these with the hyphen, without it, in lower case,
and with a space where the hyphen was; all four are the same code, and a check
that disagrees locks somebody out of their own account over punctuation.
*/
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(normaliseRecovery(code)))
	return hex.EncodeToString(sum[:])
}

func normaliseRecovery(code string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(code) {
		if strings.ContainsRune(recoveryAlphabet, r) {
			out.WriteRune(r)
		}
	}
	return out.String()
}
