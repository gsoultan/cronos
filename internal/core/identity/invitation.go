package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

/*
Invitation is a place held for somebody who has not arrived yet.

Adding a person used to mean choosing their password and telling them what it
was. That password is then in whatever channel it was sent through — a chat
message, a ticket, somebody's sent folder — and it is known to at least two
people from the moment it exists, one of whom has no way to be sure it is not
known to more. "Change it when you sign in" is advice, and advice is not a
control.

An invitation inverts it: the account has no password until the person sets one,
and the only thing that travels is a secret that stops working the moment it is
used.
*/
type Invitation struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`

	// Where they will land, decided when the invitation was written rather
	// than when it is accepted. Somebody invited to one project must not
	// arrive in another because their inviter moved in the meantime.
	Org     string `json:"org"`
	Project string `json:"project"`
	Role    string `json:"role"`

	// InvitedBy is who to ask about it. In the audit trail either way; here so
	// the person accepting can see a name they recognise rather than a link
	// from nobody, which is what a phishing email looks like.
	InvitedBy string `json:"invitedBy,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	Expires   time.Time `json:"expires"`
	// Accepted is zero until it is used, and set exactly once.
	Accepted *time.Time `json:"accepted,omitempty"`
}

/*
InvitationLife is how long one is good for.

A week. Long enough to survive somebody being on leave when it arrives, short
enough that a link forwarded into a mailing list archive in March is not a way
into the account in September.

It is not renewed on use and there is no refresh: an invitation that renews
itself is a permanent credential wearing an expiry, which is the thing this
replaced.
*/
const InvitationLife = 7 * 24 * time.Hour

// ErrInvitation is every way one can fail to be usable.
//
// One error for expired, spent, and never-existed. They are different facts and
// telling them apart is a way to learn which addresses have accounts here and
// which invitations are still open, from an endpoint that by design requires no
// session.
var ErrInvitation = errors.New("identity: this invitation cannot be used")

/*
NewInvitation mints one and the secret that opens it.

Two values, and only the first ever leaves this process in a form that can be
replayed: the secret goes in the email, the hash goes in the database. A
database that stored the secret would be a database whose backup is a set of
working invitations, and whose read-only replica is enough to become somebody
else.

256 bits from crypto/rand, so there is nothing to guess and no rate limit
standing between an attacker and an account — the limit on the accept endpoint
is there for the noise, not for the arithmetic.
*/
func NewInvitation(secretBytes int) (secret, hash string, err error) {
	if secretBytes <= 0 {
		secretBytes = 32
	}
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		// Unlike a user id, there is no safe fallback: a predictable
		// invitation secret is an open door, so this fails and nothing is
		// written.
		return "", "", err
	}

	// URL-safe and unpadded, because it travels in a link and `+`, `/` and `=`
	// all mean something else there.
	secret = base64.RawURLEncoding.EncodeToString(b)
	return secret, HashInvitation(secret), nil
}

/*
HashInvitation is how a secret is looked up.

SHA-256, not bcrypt. Password hashing is slow on purpose because a password has
perhaps forty bits of entropy and an attacker with the file can try billions;
this secret has two hundred and fifty-six from a CSPRNG, so there is nothing to
brute force and the cost would buy nothing but a slower endpoint.

What it does buy is the same thing it buys for passwords: somebody who reads the
table cannot use what they read.
*/
func HashInvitation(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// NewInvitationID names one.
func NewInvitationID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "inv_unnamed"
	}
	return "inv_" + hex.EncodeToString(b[:])
}

// Usable reports whether this invitation may still be accepted.
//
// Both conditions, in the core, so the store and the API cannot disagree about
// what expired means — and so a store that forgot one of them is a test failure
// here rather than an account somebody opened with a link from last year.
func (i Invitation) Usable(now time.Time) bool {
	return i.Accepted == nil && !now.After(i.Expires)
}
