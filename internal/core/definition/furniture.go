package definition

import "fmt"

// Furniture is a running header or footer.
//
// Template names a Template definition for the paginated renderer; Text is a
// one-line alternative with {{ .page }} and {{ .pages }} available, counted
// within each recipient's own statement. Both kinds, and not one, because a
// letterhead is a layout and a page number is a sentence — but one of them on
// any one header, since drawing both would leave neither where it was put.
type Furniture struct {
	Template string `json:"template,omitempty" yaml:"template,omitempty"`
	Text     string `json:"text,omitempty" yaml:"text,omitempty"`
}

func (f Furniture) validate(output, role string) error {
	if f.Template != "" && f.Text != "" {
		return fmt.Errorf("%w: output %q %s has both a template and text; it draws one",
			ErrInvalid, output, role)
	}
	if f.Template != "" && !slug.MatchString(f.Template) {
		return fmt.Errorf("%w: output %q %s template %q must name a Template definition",
			ErrInvalid, output, role, f.Template)
	}
	return nil
}
