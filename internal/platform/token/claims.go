package token

// Claims are what a token asserts.
//
// Short JSON names because this travels in a header on every request. Long
// enough to read in a log, short enough not to be the reason a request is
// rejected for header size.
type Claims struct {
	// Audience is what the token may be used for.
	//
	// The one field that is checked before anything else is trusted. An embed
	// token belongs to an end customer of our customer and a portal token
	// belongs to an author; without this they are the same signed blob and the
	// first is usable for the second's endpoints.
	Audience string `json:"aud"`
	// Role is the project role a portal token carries. Ignored for embed
	// tokens, which are always viewers whatever they claim.
	Role string `json:"rol,omitempty"`

	Org     string `json:"org"`
	Project string `json:"prj"`
	// Subject identifies the end user in the *host's* model, not ours. Cronos
	// never sees their user table; this is whatever they chose to put here,
	// and it exists so a run record can be traced back.
	Subject string `json:"sub"`
	/*
	   Platform marks a deployment administrator.

	   Carried in the token so every request does not have to ask the database,
	   and honoured only for a portal audience — an embed token belongs to an
	   end customer of our customer, and this is the one claim that must never
	   be reachable from there.

	   It grants nothing inside any project. Taking it away ends that account's
	   sessions rather than waiting for this claim to expire, because a
	   revocation that takes eight hours is not a revocation.
	*/
	Platform bool `json:"plt,omitempty"`
	/*
	   Enrol marks a session that may do one thing: set up a second factor.

	   Issued when a project requires one and this account has none. They signed
	   in — the password was right — and the session reaches the enrolment
	   endpoints and nothing else, so the requirement bites immediately without
	   locking anybody out of their own reporting.

	   A claim rather than a separate audience because everything else about it
	   is a portal session: the same signature, the same expiry, the same
	   subject. What differs is one bit, and the check that reads it is in one
	   place.
	*/
	Enrol bool `json:"enr,omitempty"`
	// Report pins the token to one report. Empty means any report in the
	// project, which is a decision the minting host makes rather than a
	// default we impose.
	Report string `json:"rpt,omitempty"`
	// Scope is the row-level constraint. The whole point of the token.
	Scope map[string]string `json:"scp,omitempty"`
	// Params are dataset parameters the host fixed. A caller cannot widen them.
	Params map[string]any `json:"prm,omitempty"`
	// ID names a token that can be withdrawn before it expires.
	//
	// A signed token is valid until it is not, and nothing about the signature
	// can be taken back — so revocation is a record somewhere that this one no
	// longer counts, and this is what that record is keyed by. Empty for the
	// tokens a host mints for itself: there is nothing to revoke a token by
	// that we never issued and cannot list.
	ID string `json:"jti,omitempty"`

	IssuedAt  int64 `json:"iat"`
	ExpiresAt int64 `json:"exp"`
}
