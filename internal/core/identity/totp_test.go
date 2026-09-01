package identity_test

import (
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
TOTP, against the vectors in the RFC.

Written out rather than taken from a library, so it is checked against the
document rather than against itself. RFC 6238 Appendix B publishes codes for a
known secret at known instants; if these pass, an authenticator app agrees with
this code, which is the only property that matters.

The RFC's SHA-1 vectors use the ASCII secret "12345678901234567890", which is
this in base32.
*/
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestAgainstTheRFCsOwnVectors(t *testing.T) {
	// Appendix B, the SHA-1 column, truncated to six digits — the published
	// table is eight, and the low six are what a six-digit app shows.
	for _, c := range []struct {
		at   int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	} {
		got, err := identity.TOTPCode(rfcSecret, time.Unix(c.at, 0))
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Fatalf("at %d: got %s, the RFC says %s", c.at, got, c.want)
		}
	}
}

/*
A code from the step before still works, and one from two steps ago does not.

A phone's clock drifts and people type slowly, so the previous code has to be
accepted or enrolment fails for anybody whose watch is thirty seconds out.
Accepting more than one step either side is where a second factor quietly
becomes a short password with a long life.
*/
func TestTheWindowIsOneStepEitherSide(t *testing.T) {
	secret, err := identity.NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)

	for _, offset := range []time.Duration{-identity.Step, 0, identity.Step} {
		code, err := identity.TOTPCode(secret, now.Add(offset))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := identity.CheckTOTP(secret, code, now); !ok {
			t.Fatalf("a code %s away was refused", offset)
		}
	}

	for _, offset := range []time.Duration{-2 * identity.Step, 2 * identity.Step} {
		code, err := identity.TOTPCode(secret, now.Add(offset))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := identity.CheckTOTP(secret, code, now); ok {
			t.Fatalf("a code %s away was accepted", offset)
		}
	}
}

/*
The step comes back, so the caller can spend it.

A code is good for a whole step, which means somebody who reads one off a
shoulder or a shared screen can use it again inside that window. Without a step
to record, there is nothing to compare a repeat against — and that is the
difference between a second factor and a thirty-second password.
*/
func TestTheStepIsReportedSoACodeCanBeSpent(t *testing.T) {
	secret, _ := identity.NewTOTPSecret()
	now := time.Unix(1_800_000_000, 0)

	code, _ := identity.TOTPCode(secret, now)
	step, ok := identity.CheckTOTP(secret, code, now)
	if !ok {
		t.Fatal("the current code was refused")
	}
	if step != now.Unix()/int64(identity.Step.Seconds()) {
		t.Fatalf("step %d for %s", step, now)
	}

	// The same code checked again reports the same step, which is what lets a
	// caller notice it has been used.
	if again, ok := identity.CheckTOTP(secret, code, now); !ok || again != step {
		t.Fatalf("a repeat reported step %d (ok=%v)", again, ok)
	}
}

// Anything that is not six digits is refused before any arithmetic. A caller
// passing an empty string — a form submitted with the field untouched — must
// not find that the empty code happens to match something.
func TestOnlySixDigitsAreEvenConsidered(t *testing.T) {
	secret, _ := identity.NewTOTPSecret()
	now := time.Now()

	for _, bad := range []string{"", "12345", "1234567", "abcdef", "12 34 56"} {
		if _, ok := identity.CheckTOTP(secret, bad, now); ok {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

// Whitespace either side is trimmed, because people paste codes with it and
// failing on a trailing space is a support ticket rather than a security
// property.
func TestSurroundingSpaceIsForgiven(t *testing.T) {
	secret, _ := identity.NewTOTPSecret()
	now := time.Now()

	code, _ := identity.TOTPCode(secret, now)
	if _, ok := identity.CheckTOTP(secret, " "+code+"\n", now); !ok {
		t.Fatal("a pasted code with whitespace was refused")
	}
}

// Somebody else's secret does not open this account, which is the whole point.
func TestAnotherSecretsCodeDoesNotWork(t *testing.T) {
	mine, _ := identity.NewTOTPSecret()
	theirs, _ := identity.NewTOTPSecret()
	now := time.Now()

	code, _ := identity.TOTPCode(theirs, now)
	if _, ok := identity.CheckTOTP(mine, code, now); ok {
		t.Fatal("another account's code opened this one")
	}
}

/*
The URI is one an authenticator app can read.

Not asserted by parsing it back — that would check this code against itself —
but by the shape every app documents: the scheme, the issuer in the label *and*
in a parameter, and the parameters stated rather than defaulted.
*/
func TestTheProvisioningURIIsWhatAppsExpect(t *testing.T) {
	uri := identity.TOTPURI("cronos", "dewi@acme.example", rfcSecret)

	for _, want := range []string{
		"otpauth://totp/",
		// The colon is the label separator and stays unescaped, which is the
		// form Google's own documentation shows and every app splits on.
		"cronos:dewi@acme.example",
		"secret=" + rfcSecret,
		"issuer=cronos",
		"algorithm=SHA1",
		"digits=6",
		"period=30",
	} {
		if !strings.Contains(uri, want) {
			t.Fatalf("no %q in %s", want, uri)
		}
	}
}

// Two secrets are two secrets. A generator that repeated would give everybody
// in a deployment the same second factor.
func TestEverySecretIsItsOwn(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		s, err := identity.NewTOTPSecret()
		if err != nil {
			t.Fatal(err)
		}
		if seen[s] {
			t.Fatalf("a secret repeated: %s", s)
		}
		seen[s] = true
	}
}
