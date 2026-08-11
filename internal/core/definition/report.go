package definition

// Report is what someone opens, embeds or schedules.
//
// There is no Dashboard kind. A dashboard is a report whose output is
// interactive, and the difference lives in how it is rendered rather than in
// what it is — docs/report-format.md has the argument.
type Report struct {
	Name        string `json:"name" yaml:"name"`
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Folder      string `json:"folder,omitempty" yaml:"folder,omitempty"`
	// Dataset is the default every block inherits.
	Dataset string `json:"dataset" yaml:"dataset"`
	// Params narrows the dataset's parameters for this report.
	Params  map[string]ParamOverride `json:"params,omitempty" yaml:"params,omitempty"`
	Filters []Filter                 `json:"filters,omitempty" yaml:"filters,omitempty"`
	Outputs []Output                 `json:"outputs" yaml:"outputs"`
}

// Output returns the named profile.
func (r Report) Output(name string) (Output, bool) {
	for _, o := range r.Outputs {
		if o.Name == name {
			return o, true
		}
	}
	return Output{}, false
}

// Rendered returns the first output using the given renderer.
//
// How a viewer asks for "the one a browser draws" without knowing what the
// author called it — the name is the author's, the renderer is the contract.
func (r Report) Rendered(kind RendererKind) (Output, bool) {
	for _, o := range r.Outputs {
		if o.Renderer == kind {
			return o, true
		}
	}
	return Output{}, false
}

// Datasets returns every dataset the report reads, deduplicated, in the order
// blocks first mention them.
//
// Ordered rather than a set because callers load and compile in this order,
// and two runs of the same report must produce the same statements to share a
// cache entry.
func (r Report) Datasets() []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range r.Outputs {
		for _, b := range o.Layout {
			name := b.DatasetFor(r.Dataset)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// Heading is what a viewer puts at the top.
func (r Report) Heading() string {
	if r.Title != "" {
		return r.Title
	}
	return r.Name
}

// Pinned reports whether callers may override the named parameter.
func (r Report) Pinned(param string) bool {
	return r.Params[param].Pin
}
