// Package share is a report handed to somebody who is not in the project.
//
// The token design already answers "who may see what": an embed-audience token
// carries a report, a row scope and an expiry, and the query builder makes the
// scope structural rather than advisory. A share is that token plus the one
// thing a signature cannot express — that it can be taken back.
package share

import "time"

// Share is a link somebody handed out.
//
// The token is not a field. It is signed, so it needs no storage to be valid,
// and keeping a copy would put a live credential in a table that is backed up,
// replicated, and readable by everyone who can read the others. What is stored
// is the record that says the token still counts.
type Share struct {
	ID      string `json:"id"`
	Org     string `json:"-"`
	Project string `json:"-"`

	// Report is the one report the link opens. Never empty: a share that could
	// open any report in the project is not a share of a report, and the
	// person clicking Share was looking at one.
	Report string `json:"report"`

	// Scope is what the recipient sees through, copied from the sharer at the
	// moment of sharing.
	//
	// Copied rather than referenced: a link that widened when its author's own
	// access widened would be a grant nobody made, and one that narrowed when
	// they left the company would break for a customer who did nothing wrong.
	Scope map[string]string `json:"scope,omitempty"`

	CreatedBy string     `json:"createdBy"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

// Live reports whether the share still opens.
//
// Revoked first, because it is the deliberate answer and an expired-and-also-
// revoked share should read as revoked to whoever is looking at the list.
func (s Share) Live(now time.Time) bool {
	if s.RevokedAt != nil {
		return false
	}
	return s.ExpiresAt == nil || now.Before(*s.ExpiresAt)
}

// State is what a list shows: one word, and the same word the API returns.
func (s Share) State(now time.Time) string {
	switch {
	case s.RevokedAt != nil:
		return "revoked"
	case s.ExpiresAt != nil && !now.Before(*s.ExpiresAt):
		return "expired"
	default:
		return "live"
	}
}
