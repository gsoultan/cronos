package definition

// RowScope is a predicate every query against the dataset carries, whoever
// asks.
//
// It is written against the dataset's own output columns, not the underlying
// tables, because that is the only surface an author can reason about and the
// only one that survives a rewrite of the query beneath it.
//
// Row scope is not a filter. A filter is something a caller chose and may
// remove; this is applied after the caller is finished and cannot be removed
// by anything the caller sends. See docs/tenancy.md.
type RowScope struct {
	Predicate string `json:"predicate" yaml:"predicate"`
}
