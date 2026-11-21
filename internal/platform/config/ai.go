package config

/*
AI is the model that explains reports to the people reading them.

Off unless a key is set, and that is the whole of the opt-in: an explanation
sends a reader's figures to a provider, which a deployment should choose to do
rather than discover it does. See docs/explaining.md for what is sent.
*/
type AI struct {
	// Provider names the adapter. "anthropic" is the one there is, and the
	// default; anything else is refused at startup rather than ignored.
	Provider string
	APIKey   string
	// Model is the provider's model id. Empty is the adapter's default.
	Model string
	// BaseURL points the adapter at a gateway or proxy instead of the
	// provider's own API.
	BaseURL string
}

// Configured reports whether the deployment asked for explanations.
func (a AI) Configured() bool { return a.APIKey != "" }
