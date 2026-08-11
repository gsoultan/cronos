package burst

import "context"

// Channel delivers one rendered document.
//
// Two methods, because delivery has exactly two questions: where is this
// going, and did it arrive. Name is what a schedule's `via` resolves to.
type Channel interface {
	Name() string
	Deliver(ctx context.Context, d Delivery) error
}

// Delivery is one document on its way to one recipient.
type Delivery struct {
	// To is the destination, already resolved from the row.
	To string
	// Filename is what the recipient sees. Resolved too, because a mailbox of
	// attachments all called statement.pdf is a mailbox nobody can search.
	Filename string
	Subject  string
	Body     string
	Document []byte
	// Options are the channel's own settings from the schedule.
	Options map[string]string
}
