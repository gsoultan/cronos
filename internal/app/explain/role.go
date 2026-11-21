package explain

// Role says who said a Message.
type Role string

const (
	// RoleUser is the reader.
	RoleUser Role = "user"
	// RoleAssistant is an earlier answer, as the reader was shown it.
	RoleAssistant Role = "assistant"
)
