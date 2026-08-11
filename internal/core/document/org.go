package document

// Org is the sender's identity, printed in the running header.
//
// This is the organisation the report belongs to, not the product. A statement
// is from Acme Logistics; that cronos typeset it is not the recipient's
// business.
type Org struct {
	Name string `json:"name"`
	// Logo is a path relative to the render root, or empty. It is deliberately
	// not a URL: a render that fetches is a render that can hang, and a burst
	// of 5,000 would fetch the same logo 5,000 times.
	Logo string `json:"logo,omitempty"`
}
