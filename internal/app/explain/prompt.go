package explain

// Prompt is one request to a model: standing instructions, then the turn.
type Prompt struct {
	// System is the same for every request, so a provider can cache it.
	System string
	// User carries the report and the question, delimited as data.
	User string
}
