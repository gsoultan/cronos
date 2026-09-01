package identity

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

/*
ErrReset is the only thing a redemption ever says when it will not proceed.

Expired, already spent, never issued, or belonging to an account that has since
been disabled — one error for all of them. Telling them apart would answer, to
anybody holding a string, whether that string was ever a real link and whose
account it was for.
*/
var ErrReset = errors.New("identity: this reset link cannot be used")

/*
Reset is a held-open door for somebody who cannot get in.

Until this there was none. An account with a forgotten password could be
recovered by an administrator with shell access on the server and a psql
prompt, writing a bcrypt hash by hand — cronos-user creates accounts and
deliberately will not reset one. For a product an ISV runs for their own team
that makes the commonest support request in software into an incident with
database access in it, which is both an outage for the person and a standing
reason for somebody to keep a production DSN on their laptop.

It is the same shape as an invitation and differs in the two ways that matter.
It lives for an hour rather than a week, because the person asking for it is
sitting at the sign-in page now. And using it ends every session that account
has, because "I cannot get in" and "somebody else is in" are the same sentence
from the outside, and a reset that leaves the intruder signed in has recovered
nothing.

What it deliberately does not do is stand in for the second factor. A reset
proves control of a mailbox; a second factor exists because control of a mailbox
is not enough. Signing in after one still asks for a code.
*/
type Reset struct {
	ID string `json:"id"`
	// UserID rather than an email, because the account is resolved once, when
	// the reset is asked for. Resolving it again at redemption would let an
	// address that changed in between point the reset at somebody else.
	UserID string `json:"userId"`
	// Email as it was, for the audit line. Not used to find the account.
	Email string `json:"email"`

	CreatedAt time.Time `json:"createdAt"`
	Expires   time.Time `json:"expires"`
	// Used is zero until it is spent, and set exactly once.
	Used *time.Time `json:"used,omitempty"`
}

/*
ResetLife is how long a reset link works for.

An hour. The person asking is at the sign-in page with the email open, so a
week buys them nothing and leaves a working key to the account sitting in a
mailbox — which is where an attacker who has the mailbox goes looking, and
which is exactly the window a mailbox breach a month later would otherwise
still be inside.

Not renewed on use, and there is no refresh. Asking again is free.
*/
const ResetLife = time.Hour

/*
NewReset mints the secret that travels and the hash it is found by.

The same primitive as an invitation's, called through rather than copied: 256
bits from crypto/rand, URL-safe because it goes in a link, and stored only as
its SHA-256 so that a backup of this table is not a set of working keys to
every account in it. Two implementations of that would be two things to keep
right.
*/
func NewReset() (secret, hash string, err error) {
	return NewInvitation(32)
}

// HashReset is how a reset secret is looked up. See HashInvitation.
func HashReset(secret string) string {
	return HashInvitation(secret)
}

// NewResetID names one.
func NewResetID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "rst_unnamed"
	}
	return "rst_" + hex.EncodeToString(b[:])
}

/*
Usable reports whether this reset may still be spent.

Both conditions, in the core, so the store and the API cannot disagree about
what spent means — and so a store that checked only the expiry is a test
failure here rather than a link somebody used twice.
*/
func (r Reset) Usable(now time.Time) bool {
	return r.Used == nil && !now.After(r.Expires)
}
