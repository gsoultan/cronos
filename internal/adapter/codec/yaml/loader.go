package yaml

import (
	"bytes"
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
	"gopkg.in/yaml.v3"
)

// Kind names the four artifacts a repository holds.
const (
	KindDataSource = "DataSource"
	KindDataset    = "Dataset"
	KindReport     = "Report"
	KindSchedule   = "Schedule"
)

// Loader decodes definition documents.
//
// A struct with no fields rather than package functions, so a caller depends on
// something it can substitute — a test that wants to inject a malformed
// document does not have to write a file.
type Loader struct{}

// Kind reports what a document declares, without decoding its spec.
//
// Useful to a repository walking a directory: it can route each file without
// having to try every type and see which one does not error.
func (Loader) Kind(data []byte) (string, error) {
	var e envelope
	if err := yaml.Unmarshal(data, &e); err != nil {
		return "", fmt.Errorf("%w: %v", ErrDecode, err)
	}
	if err := e.check(); err != nil {
		return "", err
	}
	return e.Kind, nil
}

// Dataset decodes a Dataset document and validates it.
//
// Decoding and validating together, because a Dataset that parsed but does not
// validate is not a value any caller wants: every one of them would have to
// remember the second call, and the one that forgets stores a broken
// definition that fails at 6am instead.
func (l Loader) Dataset(data []byte) (definition.Dataset, error) {
	e, err := l.open(data, KindDataset)
	if err != nil {
		return definition.Dataset{}, err
	}

	var ds definition.Dataset
	if err := strict(&e.Spec, &ds); err != nil {
		return definition.Dataset{}, fmt.Errorf("%w: dataset %q spec: %v",
			ErrDecode, e.Metadata.Name, err)
	}
	ds.Name = e.Metadata.Name
	ds.Description = e.Metadata.Description

	return ds, ds.Validate()
}

// Report decodes a Report document and validates it.
func (l Loader) Report(data []byte) (definition.Report, error) {
	e, err := l.open(data, KindReport)
	if err != nil {
		return definition.Report{}, err
	}

	var r definition.Report
	if err := strict(&e.Spec, &r); err != nil {
		return definition.Report{}, fmt.Errorf("%w: report %q spec: %v",
			ErrDecode, e.Metadata.Name, err)
	}
	r.Name = e.Metadata.Name
	r.Description = e.Metadata.Description
	r.Folder = e.Metadata.Folder

	return r, r.Validate()
}

func (Loader) open(data []byte, want string) (envelope, error) {
	var e envelope
	if err := yaml.Unmarshal(data, &e); err != nil {
		return envelope{}, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	if err := e.check(); err != nil {
		return envelope{}, err
	}
	if e.Kind != want {
		return envelope{}, fmt.Errorf("%w: this is a %s, not a %s", ErrDecode, e.Kind, want)
	}
	return e, nil
}

func (e envelope) check() error {
	if e.APIVersion != APIVersion {
		return fmt.Errorf("%w: apiVersion %q, want %q", ErrDecode, e.APIVersion, APIVersion)
	}
	if e.Metadata.Name == "" {
		// The name is the identity a repository stores under. Without one the
		// document has no address, and a default would collide silently.
		return fmt.Errorf("%w: metadata.name is required", ErrDecode)
	}
	return nil
}

// strict decodes a spec and refuses fields the model does not have.
//
// yaml.v3 ignores unknown keys by default, which turns `pagesize: 50` into a
// table of a hundred rows and tells nobody. A definition is configuration
// someone typed; the failure mode for a typo has to be a message, not a
// quietly different report.
//
// The cost is real: every field the format grows must be modelled here before
// a document using it will load. That is the intended pressure — the model and
// the format staying in step is the whole point.
func strict(spec *yaml.Node, into any) error {
	raw, err := yaml.Marshal(spec)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	return dec.Decode(into)
}
