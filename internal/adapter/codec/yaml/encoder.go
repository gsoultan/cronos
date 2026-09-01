package yaml

import (
	"bytes"
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
	"gopkg.in/yaml.v3"
)

// Encoder writes definitions as the documents authors edit.
//
// The direction Loader does not go. Anything that produces a definition
// programmatically — the importer, an export of a stored version, a builder
// saving to a directory — needs bytes someone will later open in an editor and
// a diff. So this is not a debug dump: the envelope is in the order the
// examples use, the indent is the repository's, and a query is a literal block
// rather than one quoted line, because the SQL is the part a person reviews.
//
// A struct with no fields, matching Loader, so a caller depends on something it
// can substitute.
type Encoder struct{}

// Indent is the nesting the format is written with.
//
// yaml.v3 defaults to four, and a definitions directory that mixes two widths
// produces a diff on every file the first time anything re-saves one.
const Indent = 2

// Dataset encodes a dataset as a cronos.dev/v1 document.
//
// Validated first. Encoding an invalid definition would produce a file that
// looks like every other one in the directory and fails on load, at which point
// the error names the file rather than whatever wrote it.
func (e Encoder) Dataset(ds definition.Dataset) ([]byte, error) {
	if err := ds.Validate(); err != nil {
		return nil, err
	}
	return e.document(KindDataset, metadata{
		Name: ds.Name, Title: ds.Title, Description: ds.Description,
	}, ds, "query")
}

// Report encodes a report as a cronos.dev/v1 document.
func (e Encoder) Report(r definition.Report) ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return e.document(KindReport, metadata{
		Name: r.Name, Title: r.Title, Description: r.Description, Folder: r.Folder,
	}, r)
}

// DataSource encodes a datasource as a cronos.dev/v1 document.
func (e Encoder) DataSource(ds definition.DataSource) ([]byte, error) {
	if err := ds.Validate(); err != nil {
		return nil, err
	}
	return e.document(KindDataSource, metadata{
		Name: ds.Name, Title: ds.Title, Description: ds.Description, Labels: ds.Labels,
	}, ds)
}

// Schedule encodes a schedule as a cronos.dev/v1 document.
func (e Encoder) Schedule(s definition.Schedule) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return e.document(KindSchedule, metadata{
		Name: s.Name, Title: s.Title, Description: s.Description,
	}, s)
}

// document assembles the envelope around an already-checked definition.
//
// The spec is encoded to a node and then edited, rather than copied into a
// struct with the identity fields blanked: those fields carry no omitempty —
// a Dataset's name is required in the domain — so blanking them would write
// `name: ""` into every spec, which strict decoding then refuses on the way
// back in. blocks names the keys to write as literal scalars.
func (Encoder) document(kind string, meta metadata, spec any, blocks ...string) ([]byte, error) {
	var body yaml.Node
	if err := body.Encode(spec); err != nil {
		return nil, fmt.Errorf("%w: %s spec: %v", ErrEncode, kind, err)
	}
	// Identity lives in metadata. Written in both places, a document would
	// carry the same string twice and disagree with itself after one edit.
	strip(&body, "name", "title", "description", "folder", "labels")
	for _, key := range blocks {
		literal(&body, key)
	}
	if len(body.Content) == 0 {
		// An empty mapping where the engine expects a spec. Validate has
		// refused every definition that could reach this, so it only guards
		// the next kind somebody adds.
		return nil, fmt.Errorf("%w: %s spec is empty", ErrEncode, kind)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(Indent)
	if err := enc.Encode(document{
		APIVersion: APIVersion, Kind: kind, Metadata: meta, Spec: &body,
	}); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrEncode, kind, err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrEncode, kind, err)
	}
	return buf.Bytes(), nil
}

// document is the envelope on the way out.
//
// Separate from envelope, which decodes: that one holds the spec as a value so
// yaml.v3 fills it in, and this one holds a pointer so a nil spec is a bug
// rather than an empty mapping.
type document struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   metadata   `yaml:"metadata"`
	Spec       *yaml.Node `yaml:"spec"`
}

// strip removes top-level keys from a mapping node.
func strip(node *yaml.Node, keys ...string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	kept := make([]*yaml.Node, 0, len(node.Content))
	// Content alternates key, value, so pairs move together or not at all.
	for i := 0; i+1 < len(node.Content); i += 2 {
		if drop[node.Content[i].Value] {
			continue
		}
		kept = append(kept, node.Content[i], node.Content[i+1])
	}
	node.Content = kept
}

// literal asks for a key's scalar to be written as a block.
//
// A request, not a guarantee: yaml.v3 falls back to a quoted scalar when the
// content cannot be represented as a block — a line ending in a space, say —
// and that is the right outcome. A block that cannot round-trip would be a
// prettier file that means something else.
func literal(node *yaml.Node, key string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != key {
			continue
		}
		if v := node.Content[i+1]; v.Kind == yaml.ScalarNode && v.Tag == "!!str" {
			v.Style = yaml.LiteralStyle
		}
		return
	}
}
