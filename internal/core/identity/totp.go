package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

/*
Time-based one-time passwords, RFC 6238.

Written out rather than taken from a library, and that is a judgement worth
stating. The algorithm is thirty lines: an HMAC of a counter, one truncation,
one modulo. A dependency for it brings a supply chain into the one part of the
product where a supply chain is the attack — and the code below is short enough
that reading it is faster than reading the library's release notes.

What is *not* obvious, and what every mistake here has in common, is that the
hard parts are not the arithmetic:

  - The window. A phone's clock drifts and a person types slowly, so a code from
    thirty seconds ago must work. Accepting too many makes the second factor a
    thirty-minute password.
  - Replay. A code is valid for a whole step, so somebody who watches one typed
    can use it again inside that step. It has to be spent.
  - Comparison. A byte-by-byte compare of the code leaks, through timing, how
    much of it was right.

SHA-1 because that is what every authenticator app implements. It is a poor
choice for signatures and a fine one here: HMAC-SHA1 has no practical weakness,
and the alternative is a secret that Google Authenticator cannot read.
*/

// Step is how long one code lasts. Thirty seconds is what every app assumes and
// is not configurable for that reason.
const Step = 30 * time.Second

// Digits is how long a code is. Six, like every app's display.
const Digits = 6

/*
Drift is how many steps either side of now are accepted.

One. That is ninety seconds of tolerance in total: the current code, the one
before it, and the one after. Enough for a phone whose clock is half a minute
out and for somebody reading digits off a screen; short enough that a code
photographed over a shoulder is stale by the time it is walked anywhere.

Zero would be correct and unusable — a person who starts typing at second 29
fails. Anything above one starts turning a second factor into a short password.
*/
const Drift = 1

// NewTOTPSecret mints one, base32 as every app expects it.
func NewTOTPSecret() (string, error) {
	// 20 bytes: the RFC's recommendation for HMAC-SHA1 and what every
	// authenticator app is tested against. More is not stronger here — the
	// output is six digits either way.
	var b [20]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// Unpadded, because the padding characters are what people mistype when
	// they enter a secret by hand instead of scanning.
	return strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString(b[:])), nil
}

/*
TOTPCode is the code for a moment.

Exported so a test can compute what an app would show, and so the enrolment
check and the sign-in check cannot drift apart by being written twice.
*/
func TOTPCode(secret string, at time.Time) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).
		DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil {
		return "", fmt.Errorf("identity: secret is not base32: %w", err)
	}

	counter := uint64(at.Unix()) / uint64(Step.Seconds())
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(message[:])
	sum := mac.Sum(nil)

	// Dynamic truncation, RFC 4226 §5.4: the low nibble of the last byte picks
	// where to read four bytes from, and the top bit is masked off so the
	// result is positive on every implementation regardless of how it treats
	// signs.
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	mod := uint32(1)
	for range Digits {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", Digits, value%mod), nil
}

/*
CheckTOTP reports whether a code is right for this moment, and which step it was.

The step comes back because a code has to be spent: one is valid for thirty
seconds, so somebody who reads it over a shoulder — or off a shared screen — can
use it again within that window. The caller records the step and refuses a
repeat, which is the difference between a second factor and a thirty-second
password.

Constant-time comparison, because the alternative leaks how many leading digits
were right and turns a million guesses into sixty.
*/
func CheckTOTP(secret, code string, at time.Time) (step int64, ok bool) {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return 0, false
	}

	for offset := -Drift; offset <= Drift; offset++ {
		when := at.Add(time.Duration(offset) * Step)
		want, err := TOTPCode(secret, when)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return when.Unix() / int64(Step.Seconds()), true
		}
	}
	return 0, false
}

/*
TOTPURI is what goes in the QR code.

The otpauth:// format every authenticator app reads. The issuer appears twice —
once as a label prefix and once as a parameter — because apps disagree about
which they honour, and one that shows "dewi@acme.example" with no product name
beside it is one somebody deletes six months later not knowing what it was for.

The secret is in this string, so it belongs in a QR code on a page and in
nothing else: not a log, not a URL somebody navigates to, not an email.
*/
func TOTPURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	query := url.Values{
		"secret": {secret},
		"issuer": {issuer},
		// Stated rather than left to the app's defaults. They agree today; a
		// URI that says what it means costs nothing and survives one of them
		// changing its mind.
		"algorithm": {"SHA1"},
		"digits":    {fmt.Sprint(Digits)},
		"period":    {fmt.Sprint(int(Step.Seconds()))},
	}
	return "otpauth://totp/" + label + "?" + query.Encode()
}
