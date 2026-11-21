package explain

// Message is one turn of a conversation.
type Message struct {
	Role Role
	Text string
}
