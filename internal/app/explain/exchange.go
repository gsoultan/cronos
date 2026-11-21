package explain

// Exchange is one earlier question in a conversation and the answer the
// reader was shown, sent back by the client so a follow-up can be read
// against it. The server keeps no conversations.
type Exchange struct {
	// Block is what the question was about, as in Request.
	Block    *int   `json:"block,omitempty"`
	Question string `json:"question,omitempty"`
	Answer   string `json:"answer"`
}
