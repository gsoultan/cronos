package paginated

// Entry is a labelled fact in a group's corner block — account number, terms,
// an invoice count.
type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
