package token

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/core/principal"
)

var at = func(s string) func() time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return func() time.Time { return t }
}

func signer(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner([]byte("a-key-that-is-long-enough-to-sign-with"))
	if err != nil {
		t.Fatal(err)
	}
	return s.WithClock(at("2026-08-11T12:00:00Z"))
}

func claims() Claims {
	return Claims{
		Audience: Embed,
		Org:      "o1", Project: "p1", Subject: "acme-user-42",
		Report: "monthly-invoice-statement",
		Scope:  map[string]string{"customer_id": "c-9"},
	}
}

func TestARoundTrip(t *testing.T) {
	s := signer(t)
	tok, err := s.Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Verify(tok, Embed)
	if err != nil {
		t.Fatalf("a token we just minted does not verify: %v", err)
	}
	if got.Scope["customer_id"] != "c-9" || got.Report != "monthly-invoice-statement" {
		t.Errorf("claims did not survive: %+v", got)
	}
	if got.ExpiresAt-got.IssuedAt != 3600 {
		t.Errorf("lifetime = %ds, want 3600", got.ExpiresAt-got.IssuedAt)
	}
}

// The claims a token carries become the identity the query compiles against.
// An embed token can never be anything but a viewer.
func TestAnEmbedTokenIsAlwaysAViewer(t *testing.T) {
	pr := claims().Principal()
	if pr.ProjectRole != principal.ProjectViewer {
		t.Errorf("role = %q, want viewer", pr.ProjectRole)
	}
	if pr.CanEdit() || pr.CanAdminProject() || pr.CanAdminOrg() {
		t.Error("an end customer of our customer must not be able to edit anything")
	}
	if pr.Scope["customer_id"] != "c-9" {
		t.Error("the scope claim is the whole point and did not arrive")
	}
}

func TestForgeriesAreRefused(t *testing.T) {
	s := signer(t)
	valid, err := s.Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(valid, ".")

	// Rewrite the scope and re-encode, leaving the signature alone. This is
	// the attack the whole design exists to stop: an end user granting
	// themselves another customer's rows.
	widened := func() string {
		c := claims()
		c.Scope = map[string]string{"customer_id": "c-1"}
		c.ExpiresAt = time.Now().Add(time.Hour).Unix()
		b, _ := json.Marshal(c)
		return parts[0] + "." + base64.RawURLEncoding.EncodeToString(b) + "." + parts[2]
	}()

	cases := map[string]string{
		"a rewritten scope":        widened,
		"a stripped signature":     parts[0] + "." + parts[1] + ".",
		"no signature at all":      parts[0] + "." + parts[1],
		"a flipped signature byte": parts[0] + "." + parts[1] + "." + flip(parts[2]),
		"a different version":      "v2." + parts[1] + "." + parts[2],
		"someone else's token":     "v1.e30.e30",
		"nothing":                  "",
	}

	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Verify(tok, Embed); !errors.Is(err, ErrInvalid) {
				t.Errorf("accepted %s: %v", name, err)
			}
		})
	}
}

// A different key must not verify, or every deployment shares a trust root.
func TestAnotherKeyDoesNotVerify(t *testing.T) {
	tok, err := signer(t).Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewSigner([]byte("a-different-key-that-is-also-long-enough"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.WithClock(at("2026-08-11T12:00:00Z")).Verify(tok, Embed); !errors.Is(err, ErrInvalid) {
		t.Errorf("a token signed elsewhere verified: %v", err)
	}
}

func TestExpiry(t *testing.T) {
	s := signer(t)
	tok, err := s.Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.WithClock(at("2026-08-11T12:59:00Z")).Verify(tok, Embed); err != nil {
		t.Errorf("still valid at 59 minutes: %v", err)
	}
	if _, err := s.WithClock(at("2026-08-11T13:05:00Z")).Verify(tok, Embed); !errors.Is(err, ErrInvalid) {
		t.Error("an expired token verified")
	}
	// Clocks disagree; a token must not die the instant one of them ticks.
	if _, err := s.WithClock(at("2026-08-11T13:00:20Z")).Verify(tok, Embed); err != nil {
		t.Errorf("20 seconds of skew should be tolerated: %v", err)
	}
}

// Treating a missing exp as "never expires" is how one leaked token stays
// useful for years.
func TestATokenWithNoExpiryIsRefused(t *testing.T) {
	s := signer(t)
	c := claims()
	b, _ := json.Marshal(c) // IssuedAt and ExpiresAt both zero
	body := "v1." + base64.RawURLEncoding.EncodeToString(b)
	tok := body + "." + base64.RawURLEncoding.EncodeToString(s.sign(body))

	if _, err := s.Verify(tok, Embed); !errors.Is(err, ErrInvalid) {
		t.Error("a correctly signed token with no expiry was accepted")
	}
}

// One error for every failure. Telling a caller which half of their forgery
// worked turns verification into an oracle.
func TestFailuresAreIndistinguishable(t *testing.T) {
	s := signer(t)
	var seen []string
	for _, tok := range []string{"", "v1.aaa.bbb", "v9.aaa.bbb", "garbage"} {
		if _, err := s.Verify(tok, Embed); err != nil {
			seen = append(seen, errors.Unwrap(err).Error())
		}
	}
	for _, e := range seen {
		if e != ErrInvalid.Error() {
			t.Errorf("failures should share one sentinel, got %q", e)
		}
	}
}

func TestMintRefuses(t *testing.T) {
	s := signer(t)
	cases := map[string]struct {
		c        Claims
		lifetime time.Duration
	}{
		// A permanent credential in a browser.
		"a lifetime beyond the maximum": {claims(), 48 * time.Hour},
		"a lifetime of nothing":         {claims(), 0},
		// Would authenticate into whatever the caller asked for.
		"no organization": {Claims{Project: "p1"}, time.Hour},
		"no project":      {Claims{Org: "o1"}, time.Hour},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Mint(c.c, c.lifetime); err == nil {
				t.Errorf("minted %s", name)
			}
		})
	}
}

// HMAC-SHA256 with a short key is HMAC-SHA256 with a guessable key, and this
// is a configuration mistake an operator should meet at startup.
func TestAShortKeyIsRefusedAtStartup(t *testing.T) {
	if _, err := NewSigner([]byte("short")); !errors.Is(err, ErrWeakKey) {
		t.Errorf("got %v, want ErrWeakKey", err)
	}
}

func flip(s string) string {
	if s == "" {
		return "x"
	}
	b := []byte(s)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	return string(b)
}

// The check the whole field exists for. Without it an embed token and a portal
// token are the same signed blob, and the first opens the second's endpoints.
func TestATokenIsUselessOutsideItsAudience(t *testing.T) {
	s := signer(t)

	embed, err := s.Mint(claims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(embed, Portal); !errors.Is(err, ErrInvalid) {
		t.Error("an embed token opened the portal audience")
	}

	author := claims()
	author.Audience, author.Role = Portal, string(principal.ProjectEditor)
	portal, err := s.Mint(author, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(portal, Embed); !errors.Is(err, ErrInvalid) {
		t.Error("a portal token was accepted as an end customer")
	}
}

// A role on an embed token is a claim an end customer wrote, and it must mean
// nothing.
func TestOnlyAPortalTokenCarriesARole(t *testing.T) {
	pretender := claims()
	pretender.Role = string(principal.ProjectAdmin)
	if pr := pretender.Principal(); pr.CanEdit() {
		t.Error("an embed token claiming admin was believed")
	}

	author := claims()
	author.Audience, author.Role = Portal, string(principal.ProjectEditor)
	if pr := author.Principal(); !pr.CanEdit() {
		t.Error("a portal editor cannot edit")
	}

	// A role nobody recognises falls back to viewer rather than to nothing.
	nonsense := claims()
	nonsense.Audience, nonsense.Role = Portal, "superuser"
	if pr := nonsense.Principal(); pr.CanEdit() || !pr.CanRead() {
		t.Errorf("an unknown role became %q", pr.ProjectRole)
	}
}

// A token with no audience predates the field, and a missing claim must not
// read as "any".
func TestATokenWithNoAudienceIsRefused(t *testing.T) {
	s := signer(t)
	c := claims()
	c.Audience = ""
	if _, err := s.Mint(c, time.Hour); err == nil {
		t.Error("minted a token with no audience")
	}
}

// The exemption from row scope is the one thing an end customer must never be
// able to claim, so it comes from the audience rather than from a field.
func TestOnlyAPortalTokenIsAProjectMember(t *testing.T) {
	if claims().Principal().Member {
		t.Error("an embed token claimed to be a project member")
	}
	author := claims()
	author.Audience = Portal
	if !author.Principal().Member {
		t.Error("a portal token is not a project member")
	}
}

/*
TestNoTokenCarriesAnOrgRole pins a tier that is defined and not wired.

principal has org roles beside project roles, and effective() promotes an org
owner or admin to ProjectAdmin — in whatever project the request names, with no
membership in it. Nothing in this build ever sets OrgRole, so that promotion
cannot fire and CanAdminOrg is always false. It fails closed, which is why it
is not a hole today.

The day somebody implements the tier, the obvious place is here: Claims has one
role field, and reading it into OrgRole as well is a one-line change that reads
like a fix. It would grant ProjectAdmin across every project in the
organization, from a claim, and for an embed token that claim belongs to an end
customer of a customer. So this fails rather than lets that arrive quietly —
the tier needs a source of authority that a token is not, and a test that says
what it grants.

OrgOwner and OrgAdmin are also the same strings as the project roles, so a
claim of "admin" is already believed as one thing and must not become two.
*/
func TestNoTokenCarriesAnOrgRole(t *testing.T) {
	for _, audience := range []string{Embed, Portal} {
		for _, claimed := range []string{
			string(principal.OrgOwner), string(principal.OrgAdmin),
			string(principal.OrgMember), string(principal.ProjectAdmin),
		} {
			c := claims()
			c.Audience, c.Role = audience, claimed
			pr := c.Principal()

			if pr.OrgRole != principal.None {
				t.Errorf("%s token claiming %q became org %q",
					audience, claimed, pr.OrgRole)
			}
			if pr.CanAdminOrg() {
				t.Errorf("%s token claiming %q administers the organization",
					audience, claimed)
			}
		}
	}

	// And an embed token gets nothing from the claim at all, which is the
	// boundary the rest of this file exists to hold.
	end := claims()
	end.Role = string(principal.OrgOwner)
	if pr := end.Principal(); pr.CanAdminProject() || pr.CanEdit() {
		t.Error("an end customer claiming org owner was let into the project")
	}
}

/*
TestOnlyAPortalTokenCarriesAnOrgRole is the boundary on the field that does
carry one.

An org role promotes to ProjectAdmin in whatever project the request names,
with no membership in it — so of the claims here it is the most valuable to
forge and the one whose audience check matters most. An embed token is minted
by a host application for one of its customers; if this were honoured there, a
customer's customer would administer every project that customer owns.

The unknown-word case is not pedantry either. `admin` is both a project role
and an org role, and a build that took the field as written would promote on
any string a later version might add.
*/
func TestOnlyAPortalTokenCarriesAnOrgRole(t *testing.T) {
	forged := claims()
	forged.OrgRole = string(principal.OrgOwner)
	if pr := forged.Principal(); pr.OrgRole != principal.None || pr.CanAdminOrg() {
		t.Errorf("an embed token claiming org owner became %q", pr.OrgRole)
	}
	if pr := forged.Principal(); pr.CanAdminProject() {
		t.Error("and it reached a project it is not a member of")
	}

	granted := claims()
	granted.Audience, granted.OrgRole = Portal, string(principal.OrgAdmin)
	pr := granted.Principal()
	if !pr.CanAdminOrg() {
		t.Error("a portal token granted org admin does not administer its organization")
	}
	// The whole point of the tier: into a project without a membership in it.
	if !pr.CanAdminProject() {
		t.Error("and it cannot enter a project, which is what the role is for")
	}

	// A word this build does not know is no grant, not an unrecognised one.
	odd := claims()
	odd.Audience, odd.OrgRole = Portal, "superowner"
	if pr := odd.Principal(); pr.OrgRole != principal.None || pr.CanAdminOrg() {
		t.Errorf("an unknown org role became %q", pr.OrgRole)
	}

	// An org member administers nothing, and is still not a project admin by
	// virtue of belonging to the organization.
	member := claims()
	member.Audience, member.OrgRole = Portal, string(principal.OrgMember)
	if pr := member.Principal(); pr.CanAdminOrg() || pr.CanAdminProject() {
		t.Error("an org member was promoted")
	}
}
