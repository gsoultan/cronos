package definition

import (
	"fmt"
	"regexp"
)

// Template is a Typst file a paginated output draws its running header or
// footer with: a letterhead, a logo, a line of small print.
//
// A definition of its own rather than a file beside the report, so it is
// versioned, published and stored exactly as everything else is, and one
// letterhead serves every report that names it. Its source defines
// `header(ctx)`, `footer(ctx)` or both — see docs/report-format.md.
type Template struct {
	Name        string `json:"name" yaml:"-"`
	Title       string `json:"title,omitempty" yaml:"-"`
	Description string `json:"description,omitempty" yaml:"-"`
	Source      string `json:"source" yaml:"source"`
}

// MaxTemplateSource bounds a template. A letterhead with an SVG logo inline is
// tens of kilobytes; one in megabytes is an image that belongs in a font
// directory or nowhere, and it is copied into every render's root.
const MaxTemplateSource = 256 << 10

// packageImport matches a Typst package specification, "@preview/name:1.0.0".
// A package is fetched from the network the first time it is imported, and a
// typesetter running inside a server must never reach out to anything — so a
// template that names one is refused when it is stored. The compiler is also
// cut off from the network, for the import this cannot see.
var packageImport = regexp.MustCompile(`"@[A-Za-z]`)

// Validate reports every reason the template cannot be stored.
func (t Template) Validate() error {
	switch {
	case !slug.MatchString(t.Name):
		return fmt.Errorf("%w: name %q must be lowercase letters, digits and dashes", ErrInvalid, t.Name)
	case t.Source == "":
		return fmt.Errorf("%w: template %q has no source", ErrInvalid, t.Name)
	case len(t.Source) > MaxTemplateSource:
		return fmt.Errorf("%w: template %q is %d KB, and a template may be %d KB",
			ErrInvalid, t.Name, len(t.Source)>>10, MaxTemplateSource>>10)
	case packageImport.MatchString(t.Source):
		return fmt.Errorf("%w: template %q imports a Typst package, which would be fetched "+
			"from the network; copy what it needs into the template", ErrInvalid, t.Name)
	}
	return nil
}

// Defines reports whether the source declares the function a role calls:
// header(ctx) for a header, footer(ctx) for a footer. Checked when a report
// naming the template is stored, so a letterhead used as a footer is an error
// the author sees rather than a failed render at six in the morning.
func (t Template) Defines(role string) bool {
	return regexp.MustCompile(`#let\s+` + regexp.QuoteMeta(role) + `\s*\(`).MatchString(t.Source)
}
