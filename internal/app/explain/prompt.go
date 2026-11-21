package explain

// Prompt is one request to a model: standing instructions, then the
// conversation so far, ending on the reader's newest question.
type Prompt struct {
	// System is the same for every request, so a provider can cache it.
	System string
	// Turns alternate, starting and ending with RoleUser. The first carries
	// the report, delimited as data, so it is the same prefix on every
	// request of a conversation and a provider can cache that too.
	Turns []Message
}
