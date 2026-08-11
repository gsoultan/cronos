package definition

// Furniture is a running header or footer.
//
// Template names a Typst file for the paginated renderer; Text is a one-line
// alternative with {{ .page }} and {{ .pages }} available. Both, and not one,
// because a letterhead is a layout and a page number is a sentence.
type Furniture struct {
	Template string `json:"template,omitempty" yaml:"template,omitempty"`
	Text     string `json:"text,omitempty" yaml:"text,omitempty"`
}
