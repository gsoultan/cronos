package explain

// Answer is the explanation, and what it was about.
type Answer struct {
	Text string `json:"text"`
	// Block echoes the block explained, so a panel that sent two requests
	// knows which answer is which.
	Block *int `json:"block,omitempty"`
}
