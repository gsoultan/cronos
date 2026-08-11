package definition

import (
	"fmt"
	"regexp"
)

// slug is the shape of a definition's name: what appears in a URL and a
// folder.
var slug = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// identifier is the shape of a param or field name.
//
// These end up next to SQL — a field name reaches an ORDER BY, a param name
// reaches an error message a caller sees. Constraining them to something that
// cannot be anything else closes the door here, once, rather than at each of
// the places that later handle them.
var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Validate reports every reason the dataset cannot be stored.
//
// It returns on the first, deliberately: an author fixing definitions wants
// the first real problem, not a list in which the second through fifth are
// consequences of the first.
func (d Dataset) Validate() error {
	if !slug.MatchString(d.Name) {
		return fmt.Errorf("%w: name %q must be lowercase letters, digits and dashes", ErrInvalid, d.Name)
	}
	if d.Query == "" {
		return fmt.Errorf("%w: dataset %q has no query", ErrInvalid, d.Name)
	}
	if err := d.validateSources(); err != nil {
		return err
	}
	if err := d.validateParams(); err != nil {
		return err
	}
	if err := d.validateFields(); err != nil {
		return err
	}
	return d.validateRowScope()
}

func (d Dataset) validateSources() error {
	if len(d.Sources) == 0 {
		return fmt.Errorf("%w: dataset %q names no source", ErrInvalid, d.Name)
	}
	seen := map[string]bool{}
	for _, s := range d.Sources {
		switch {
		case !slug.MatchString(s.Ref):
			return fmt.Errorf("%w: source %q is not a datasource name", ErrInvalid, s.Ref)
		case s.As != "" && !identifier.MatchString(s.As):
			// The alias is what the query writes, so it has to be something a
			// query can write.
			return fmt.Errorf("%w: source %q is aliased to %q, which is not an identifier",
				ErrInvalid, s.Ref, s.As)
		case seen[s.Name()]:
			// Two sources under one name means the query says one thing and
			// means whichever the planner picked.
			return fmt.Errorf("%w: two sources are both called %q", ErrInvalid, s.Name())
		}
		seen[s.Name()] = true
	}
	return nil
}

func (d Dataset) validateParams() error {
	seen := map[string]bool{}
	for _, p := range d.Params {
		switch {
		case !identifier.MatchString(p.Name):
			return fmt.Errorf("%w: param %q must be lowercase letters, digits and underscores", ErrInvalid, p.Name)
		case seen[p.Name]:
			return fmt.Errorf("%w: param %q declared twice", ErrInvalid, p.Name)
		case !p.Type.Valid():
			return fmt.Errorf("%w: param %q has unknown type %q", ErrInvalid, p.Name, p.Type)
		case p.Type == Enum && len(p.Values) == 0:
			// An enum with no values constrains nothing, which is the reverse
			// of what declaring an enum was meant to do.
			return fmt.Errorf("%w: enum param %q lists no values", ErrInvalid, p.Name)
		case p.Type != Enum && len(p.Values) > 0:
			return fmt.Errorf("%w: param %q is %s, so values are not applied", ErrInvalid, p.Name, p.Type)
		}
		seen[p.Name] = true
	}
	return nil
}

func (d Dataset) validateFields() error {
	if len(d.Fields) == 0 {
		return fmt.Errorf("%w: dataset %q declares no fields", ErrInvalid, d.Name)
	}
	seen := map[string]bool{}
	for _, f := range d.Fields {
		switch {
		case !identifier.MatchString(f.Name):
			return fmt.Errorf("%w: field %q must be lowercase letters, digits and underscores", ErrInvalid, f.Name)
		case seen[f.Name]:
			return fmt.Errorf("%w: field %q declared twice", ErrInvalid, f.Name)
		case !f.Role.Valid():
			return fmt.Errorf("%w: field %q has role %q, want dimension or measure", ErrInvalid, f.Name, f.Role)
		case f.Role == Measure && f.Aggregate == "":
			// Without one, every report using this field has to invent an
			// aggregate, and two reports will invent different ones.
			return fmt.Errorf("%w: measure %q declares no aggregate", ErrInvalid, f.Name)
		}
		seen[f.Name] = true
	}
	return d.validateCurrencyRefs(seen)
}

func (d Dataset) validateCurrencyRefs(known map[string]bool) error {
	for _, f := range d.Fields {
		if f.CurrencyField != "" && !known[f.CurrencyField] {
			return fmt.Errorf("%w: measure %q takes its currency from %q, which is not a field",
				ErrInvalid, f.Name, f.CurrencyField)
		}
	}
	return nil
}

func (d Dataset) validateRowScope() error {
	for i, s := range d.RowLevelSecurity {
		if s.Predicate == "" {
			// An empty predicate reads as "no restriction" and would silently
			// widen every query. A dataset that wants no row scope omits the
			// section; it does not declare an empty one.
			return fmt.Errorf("%w: row scope %d has an empty predicate", ErrInvalid, i)
		}
	}
	return nil
}
