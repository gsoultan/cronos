package definition

// Dataset is a governed query: one statement, the questions it accepts, the
// fields it returns, and the rows each caller may see.
//
// Reports bind to datasets and never to a source directly. That indirection is
// the governance: row scope, parameter types and field semantics are declared
// once here rather than re-argued in every report.
type Dataset struct {
	Name string `json:"name" yaml:"name"`
	// Title is what a person calls it. The name is an identifier — stable,
	// referenced by other definitions, awkward to change once anything points
	// at it — and those are two different jobs for one string to hold.
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Sources are the datasources the query may reference, and what it calls
	// them.
	Sources []SourceRef `json:"sources" yaml:"sources"`
	// Query is SQL with {{ .params.x }} holes. Holes become bind arguments;
	// nothing in this string is ever assembled from caller input.
	Query  string  `json:"query" yaml:"query"`
	Params []Param `json:"params,omitempty" yaml:"params,omitempty"`
	Fields []Field `json:"fields" yaml:"fields"`
	// RowLevelSecurity is applied to every execution, unconditionally.
	RowLevelSecurity []RowScope `json:"rowLevelSecurity,omitempty" yaml:"rowLevelSecurity,omitempty"`
}

// Param returns the named parameter and whether it exists.
func (d Dataset) Param(name string) (Param, bool) {
	for _, p := range d.Params {
		if p.Name == name {
			return p, true
		}
	}
	return Param{}, false
}

// Field returns the named field and whether it exists.
func (d Dataset) Field(name string) (Field, bool) {
	for _, f := range d.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// Heading is what a catalog puts in the list.
func (d Dataset) Heading() string {
	if d.Title != "" {
		return d.Title
	}
	return d.Name
}
