package history

// Status is how a run or a delivery ended.
type Status string

const (
	// Running means started and not yet finished — or the process died.
	Running Status = "running"
	// Delivered means every recipient got every channel.
	Delivered Status = "delivered"
	// Partial means some did not. Its own status rather than a success with a
	// footnote: 4,997 of 5,000 has three customers without an invoice, and
	// anything that reads this needs to be able to find them.
	Partial Status = "partial"
	// Failed means the run did not produce anything.
	Failed Status = "failed"
)
