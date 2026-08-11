package token

// Claims are what a token asserts.
//
// Short JSON names because this travels in a header on every request. Long
// enough to read in a log, short enough not to be the reason a request is
// rejected for header size.
type Claims struct {
	Org     string `json:"org"`
	Project string `json:"prj"`
	// Subject identifies the end user in the *host's* model, not ours. Cronos
	// never sees their user table; this is whatever they chose to put here,
	// and it exists so a run record can be traced back.
	Subject string `json:"sub"`
	// Report pins the token to one report. Empty means any report in the
	// project, which is a decision the minting host makes rather than a
	// default we impose.
	Report string `json:"rpt,omitempty"`
	// Scope is the row-level constraint. The whole point of the token.
	Scope map[string]string `json:"scp,omitempty"`
	// Params are dataset parameters the host fixed. A caller cannot widen them.
	Params map[string]any `json:"prm,omitempty"`

	IssuedAt  int64 `json:"iat"`
	ExpiresAt int64 `json:"exp"`
}
