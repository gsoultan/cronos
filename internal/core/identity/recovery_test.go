package identity_test

import (
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
Recovery codes are passwords, and are the reason a second factor is safe to
turn on.

Without them, enabling one is a way to lose an account — and the realistic
fallback is an administrator disabling somebody's second factor over chat, which
is a social-engineering path straight past the thing that was just enabled.
*/

func TestTheCodesShownAreTheCodesStored(t *testing.T) {
	codes, hashes, err := identity.NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != identity.RecoveryCodes || len(hashes) != len(codes) {
		t.Fatalf("%d codes, %d hashes", len(codes), len(hashes))
	}

	// The failure this guards is showing one set and storing another: ten
	// strings that open nothing, discovered on the day one is needed.
	for i, code := range codes {
		if identity.HashRecoveryCode(code) != hashes[i] {
			t.Fatalf("code %d does not hash to the stored value", i)
		}
	}
}

// A code is never stored in a form that can be used. Somebody who reads the
// table has ten dead strings.
func TestTheCodeItselfIsNotTheHash(t *testing.T) {
	codes, hashes, _ := identity.NewRecoveryCodes()

	for i, code := range codes {
		if strings.Contains(hashes[i], code) || hashes[i] == code {
			t.Fatalf("the stored value contains the code: %s", hashes[i])
		}
	}
}

/*
How somebody types it is not the question.

These are read off a screenshot or a piece of paper, months later, usually by
somebody already locked out. Hyphen, no hyphen, lower case, a space where the
hyphen was — all the same code. A check that disagrees locks somebody out of
their own account over punctuation.
*/
func TestPunctuationAndCaseDoNotMatter(t *testing.T) {
	codes, _, _ := identity.NewRecoveryCodes()
	want := identity.HashRecoveryCode(codes[0])

	for _, typed := range []string{
		strings.ToLower(codes[0]),
		strings.ReplaceAll(codes[0], "-", ""),
		strings.ReplaceAll(codes[0], "-", " "),
		" " + codes[0] + " ",
		strings.ToLower(strings.ReplaceAll(codes[0], "-", "")),
	} {
		if identity.HashRecoveryCode(typed) != want {
			t.Fatalf("%q was treated as a different code", typed)
		}
	}
}

/*
The alphabet has nothing anybody has to squint at.

No 0/O, no 1/I/L. A character somebody cannot tell apart is a code that does not
work for a reason they cannot see, at the moment they are least able to work it
out.
*/
func TestTheAlphabetHasNoAmbiguousCharacters(t *testing.T) {
	codes, _, _ := identity.NewRecoveryCodes()

	for _, code := range codes {
		if strings.ContainsAny(code, "OILU") {
			t.Fatalf("%q contains a character people misread", code)
		}
		// And the shape: two groups of five, so nobody loses their place.
		if len(code) != 11 || code[5] != '-' {
			t.Fatalf("%q is not five-hyphen-five", code)
		}
	}
}

// Ten different codes, and a different ten every time. A generator that
// repeated would hand two people the same way into each other's accounts.
func TestEveryCodeIsItsOwn(t *testing.T) {
	seen := map[string]bool{}
	for range 20 {
		codes, _, _ := identity.NewRecoveryCodes()
		for _, code := range codes {
			if seen[code] {
				t.Fatalf("a code repeated: %s", code)
			}
			seen[code] = true
		}
	}
	if len(seen) != 20*identity.RecoveryCodes {
		t.Fatalf("%d distinct codes from %d", len(seen), 20*identity.RecoveryCodes)
	}
}

// An empty string is not a recovery code. A form submitted with the field
// untouched must not hash to something a stored row could match.
func TestAnEmptyCodeIsNotSomebodysCode(t *testing.T) {
	codes, hashes, _ := identity.NewRecoveryCodes()
	empty := identity.HashRecoveryCode("")

	for i := range codes {
		if hashes[i] == empty {
			t.Fatal("an empty code matches a stored one")
		}
	}
}
