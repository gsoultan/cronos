package explain

import "context"

// Model answers one prompt with text. A provider adapter implements it.
type Model interface {
	Complete(ctx context.Context, p Prompt) (string, error)
}
