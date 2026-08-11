package definition

import "fmt"

// Filter is one control on a report's filter bar.
//
// A report's blocks may read different datasets, so a filter spanning them has
// to say what it means in each. Bind is that mapping and it is explicit
// because guessing is how a filter silently applies to half a screen.
//
// A dataset with no entry in Bind is **unaffected** by the filter. That is a
// legitimate outcome, not a mistake — a Period filter has nothing to say to a
// dataset of current stock levels — and the interface is required to show it
// on the block rather than let it be discovered.
type Filter struct {
	Name  string    `json:"name" yaml:"name"`
	Label string    `json:"label,omitempty" yaml:"label,omitempty"`
	Type  ParamType `json:"type" yaml:"type"`
	// Bind maps a dataset name to the field this filter narrows in it.
	Bind map[string]string `json:"bind" yaml:"bind"`
	// Values enumerates the permitted values. Enum only.
	Values []string `json:"values,omitempty" yaml:"values,omitempty"`
}

// Binds returns the field this filter narrows in the named dataset, and
// whether it reaches that dataset at all.
//
// The two-value form is deliberate. A caller that wants to apply the filter
// and a caller that wants to tell someone the filter does not apply here are
// asking the same question, and neither should have to reach into the map and
// decide what an empty string means.
func (f Filter) Binds(dataset string) (field string, ok bool) {
	field, ok = f.Bind[dataset]
	return field, ok && field != ""
}

// Validate reports whether the filter is well formed on its own terms.
//
// It cannot check that a bound field exists — that needs the datasets, which
// this package deliberately does not reach for. query.CheckFilters does it
// where both are in hand.
func (f Filter) Validate() error {
	switch {
	case !identifier.MatchString(f.Name):
		return fmt.Errorf("%w: filter %q must be lowercase letters, digits and underscores",
			ErrInvalid, f.Name)
	case !f.Type.Valid():
		return fmt.Errorf("%w: filter %q has unknown type %q", ErrInvalid, f.Name, f.Type)
	case f.Type == Enum && len(f.Values) == 0:
		return fmt.Errorf("%w: enum filter %q lists no values", ErrInvalid, f.Name)
	case len(f.Bind) == 0:
		// A filter bound to nothing is a control that does nothing, shown to
		// someone who will reasonably expect it to work.
		return fmt.Errorf("%w: filter %q binds to no dataset", ErrInvalid, f.Name)
	}
	return f.validateBindings()
}

func (f Filter) validateBindings() error {
	for dataset, field := range f.Bind {
		if !slug.MatchString(dataset) {
			return fmt.Errorf("%w: filter %q binds to %q, which is not a dataset name",
				ErrInvalid, f.Name, dataset)
		}
		// The field reaches SQL as text rather than as an argument, so it is
		// constrained to something that cannot be anything else.
		if !identifier.MatchString(field) {
			return fmt.Errorf("%w: filter %q binds %s to %q, which is not a field name",
				ErrInvalid, f.Name, dataset, field)
		}
	}
	return nil
}
