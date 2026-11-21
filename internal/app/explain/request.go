package explain

// Request is what a reader asked about the view they have.
type Request struct {
	// Block focuses the answer on one block, by its place in the view from
	// zero. Nil explains the whole report.
	Block *int
	// Question is the reader's own, in their words. Empty asks for a summary.
	Question string
}
