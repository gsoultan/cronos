package definition

// PageSpec is the paper a paginated output prints on, as the author writes it.
//
// Margins is a string here — "20mm" — because that is the format. It becomes a
// number in the renderer, where a bad unit is an error message rather than
// something the typesetter has to `eval`. See internal/adapter/render/paginated.
type PageSpec struct {
	Size        string `json:"size,omitempty" yaml:"size,omitempty"`
	Orientation string `json:"orientation,omitempty" yaml:"orientation,omitempty"`
	Margins     string `json:"margins,omitempty" yaml:"margins,omitempty"`
}
