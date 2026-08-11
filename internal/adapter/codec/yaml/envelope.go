package yaml

import "gopkg.in/yaml.v3"

// envelope is the outer document, common to every kind.
//
// Spec is held unparsed until the kind is known, so a Dataset's fields are
// never decoded against a Report's shape — which would silently drop
// everything rather than say the kind was wrong.
type envelope struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   metadata  `yaml:"metadata"`
	Spec       yaml.Node `yaml:"spec"`
}

// metadata is a definition's identity, which lives outside the spec because it
// is what the repository indexes rather than what the engine executes.
type metadata struct {
	Name string `yaml:"name"`
	// Title is what a person calls it, where the name is the identifier other
	// definitions point at. Every kind has one: an author who typed a name
	// into a form and had it silently discarded on save would find the field
	// blank the next time they opened it.
	Title       string            `yaml:"title"`
	Description string            `yaml:"description"`
	Folder      string            `yaml:"folder"`
	Labels      map[string]string `yaml:"labels"`
}
