package definition

// SourceRef names a datasource a dataset may read, and what the query calls it.
//
// A struct rather than a bare name because federation needs the alias: two
// datasets joining the same warehouse under different names, or a lake mounted
// as `events` so the query does not carry the connection's own naming.
type SourceRef struct {
	Ref string `json:"ref" yaml:"ref"`
	// As is the name the query uses. Empty means Ref.
	As string `json:"as,omitempty" yaml:"as,omitempty"`
}

// Name is what the query writes.
func (s SourceRef) Name() string {
	if s.As != "" {
		return s.As
	}
	return s.Ref
}
