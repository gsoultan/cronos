package history

import "time"

// Delivery is one document arriving somewhere, or not.
//
// One row per recipient per channel, because "the burst succeeded" and "this
// customer's email bounced while their archive copy was written" are both true
// at once, and only the second answers the question anybody actually asks.
type Delivery struct {
	RunID string `json:"runId"`
	// Recipient identifies the row a document was rendered for.
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	// Destination is where it went, as the channel resolved it. Recorded
	// because "we emailed them" is not an answer; the address is.
	Destination string `json:"destination"`
	Filename    string `json:"filename,omitempty"`

	Status   Status    `json:"status"`
	Attempts int       `json:"attempts"`
	Bytes    int       `json:"bytes"`
	Error    string    `json:"error,omitempty"`
	At       time.Time `json:"at"`
}
