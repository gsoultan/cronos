package definition

import "fmt"

// Validate reports every reason the report cannot be stored.
//
// It cannot check that a block's field exists — that needs the datasets, which
// this package deliberately does not reach for. query.CheckReport does it
// where both are in hand.
func (r Report) Validate() error {
	if !slug.MatchString(r.Name) {
		return fmt.Errorf("%w: name %q must be lowercase letters, digits and dashes", ErrInvalid, r.Name)
	}
	if len(r.Outputs) == 0 {
		return fmt.Errorf("%w: report %q has no outputs", ErrInvalid, r.Name)
	}
	for _, f := range r.Filters {
		if err := f.Validate(); err != nil {
			return err
		}
	}
	for param := range r.Params {
		if _, ok := r.paramNamed(param); !ok {
			return fmt.Errorf("%w: report %q overrides param %q, which is not an identifier",
				ErrInvalid, r.Name, param)
		}
	}
	return r.validateOutputs()
}

func (r Report) paramNamed(name string) (string, bool) {
	return name, identifier.MatchString(name)
}

func (r Report) validateOutputs() error {
	seen := map[string]bool{}
	for _, o := range r.Outputs {
		switch {
		case o.Name == "":
			return fmt.Errorf("%w: report %q has an output with no name", ErrInvalid, r.Name)
		case seen[o.Name]:
			return fmt.Errorf("%w: report %q has two outputs called %q", ErrInvalid, r.Name, o.Name)
		case !o.Renderer.Valid():
			return fmt.Errorf("%w: output %q has renderer %q, want interactive, paginated or spreadsheet",
				ErrInvalid, o.Name, o.Renderer)
		}
		seen[o.Name] = true
		if err := o.validate(r.Dataset); err != nil {
			return err
		}
	}
	return nil
}

func (o Output) validate(reportDefault string) error {
	if o.Renderer == Spreadsheet {
		if len(o.Sheets) == 0 {
			return fmt.Errorf("%w: spreadsheet output %q has no sheets", ErrInvalid, o.Name)
		}
		return nil
	}
	if len(o.Layout) == 0 {
		// An output with no layout renders an empty document, which looks like
		// a report that found no data rather than one that was never written.
		return fmt.Errorf("%w: output %q has an empty layout", ErrInvalid, o.Name)
	}
	for i, b := range o.Layout {
		if err := b.validate(o.Name, i, reportDefault); err != nil {
			return err
		}
	}
	return nil
}

func (b Block) validate(output string, i int, reportDefault string) error {
	switch {
	case !b.Kind.Valid():
		return fmt.Errorf("%w: %s block %d has kind %q, want stat, chart, table or text",
			ErrInvalid, output, i, b.Kind)
	case b.Kind != TextBlock && b.DatasetFor(reportDefault) == "":
		// Nothing to read. Rendering it empty would look like no data.
		return fmt.Errorf("%w: %s block %d names no dataset and the report has no default",
			ErrInvalid, output, i)
	case b.PageSize < 0:
		return fmt.Errorf("%w: %s block %d has a negative pageSize", ErrInvalid, output, i)
	}
	return b.validateShape(output, i)
}

// validateShape checks the fields each kind actually reads. A stat with no
// field renders a blank tile rather than failing, which is the worst outcome:
// it looks like an answer.
func (b Block) validateShape(output string, i int) error {
	switch b.Kind {
	case StatBlock:
		if b.Value.Field == "" {
			return fmt.Errorf("%w: %s stat %d measures no field", ErrInvalid, output, i)
		}
	case ChartBlock:
		if b.Chart == "" {
			return fmt.Errorf("%w: %s chart %d does not say what kind of chart", ErrInvalid, output, i)
		}
		if b.X.Field == "" || b.Y.Field == "" {
			return fmt.Errorf("%w: %s chart %d needs both x and y", ErrInvalid, output, i)
		}
	case TableBlock:
		if len(b.Columns) == 0 {
			return fmt.Errorf("%w: %s table %d lists no columns", ErrInvalid, output, i)
		}
	case TextBlock:
		if b.Text == "" {
			return fmt.Errorf("%w: %s text %d has no text", ErrInvalid, output, i)
		}
	}
	return nil
}
