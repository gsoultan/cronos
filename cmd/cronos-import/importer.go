package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gsoultan/cronos/internal/adapter/codec/jrxml"
	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/core/definition"
)

type importer struct {
	from    jrxml.Importer
	out     string
	share   bool
	force   bool
	verbose bool

	// names keeps definition names unique across the whole run. Two files
	// called Sales Summary and sales_summary land on one name, and the second
	// would otherwise overwrite the first.
	names taken
	// datasets maps a dataset's content to the name already written for it, so
	// forty reports over one query import one dataset rather than forty.
	datasets map[string]string

	files, wrote, shared, blocked, review, renamed int
	// unscoped counts datasets imported with no row-level security, which is all
	// of them: a .jrxml has none to carry. Counted rather than assumed so the
	// warning stops being printed if the importer ever learns to emit one.
	unscoped int
}

func (im *importer) walk(paths []string) error {
	found, err := collect(paths)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return errors.New("no .jrxml files found")
	}
	for _, path := range found {
		if err := im.one(path); err != nil {
			return err
		}
	}
	im.summarise()
	return nil
}

// one imports a single file and reports it.
func (im *importer) one(path string) error {
	im.files++
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	res, err := im.from.Import(data)
	if err != nil {
		// A refusal still has findings — the census ran — so the reason is
		// printed with everything else that was in the file.
		im.blocked++
		im.report(path, res, err)
		return nil
	}
	if res.Blocked() {
		im.blocked++
	}
	im.review += res.Needs(jrxml.Review)

	if err := im.write(&res); err != nil {
		return err
	}
	im.report(path, res, nil)
	return nil
}

// write emits the definitions, sharing a dataset when an identical one has
// already been written.
func (im *importer) write(res *jrxml.Result) error {
	if !res.HasDataset() {
		return nil
	}
	name, reused, err := im.dataset(res.Dataset)
	if err != nil {
		return err
	}
	res.Dataset.Name = name
	if reused {
		im.shared++
	} else if len(res.Dataset.RowLevelSecurity) == 0 {
		// Counted per dataset written, not per file read: forty reports sharing
		// one query are one dataset to add a predicate to, and saying forty
		// would make the number meaningless.
		im.unscoped++
	}

	if res.HasReport() {
		// The report has to point at whatever the dataset ended up called.
		res.Report.Dataset = name
		var renamed bool
		res.Report.Name, renamed = im.names.pick("reports", res.Report.Name)
		if renamed {
			im.renamed++
		}
		raw, err := (codec.Encoder{}).Report(res.Report)
		if err != nil {
			return fmt.Errorf("report %q: %w", res.Report.Name, err)
		}
		if err := im.file("reports", res.Report.Name, raw); err != nil {
			return err
		}
	}
	return nil
}

// dataset returns the name to use for this dataset, writing it if it is new.
func (im *importer) dataset(ds definition.Dataset) (string, bool, error) {
	var key string
	if im.share {
		k, err := fingerprint(ds)
		if err != nil {
			return "", false, err
		}
		if existing, seen := im.datasets[k]; seen {
			return existing, true, nil
		}
		key = k
	}

	name, renamed := im.names.pick("datasets", ds.Name)
	if renamed {
		im.renamed++
	}
	ds.Name = name
	raw, err := (codec.Encoder{}).Dataset(ds)
	if err != nil {
		return "", false, fmt.Errorf("dataset %q: %w", name, err)
	}
	if err := im.file("datasets", name, raw); err != nil {
		return "", false, err
	}
	// Recorded only once it is on disk, so a failed write cannot leave a later
	// report pointing at a dataset that was never created.
	if key != "" {
		if im.datasets == nil {
			im.datasets = map[string]string{}
		}
		im.datasets[key] = name
	}
	return name, false, nil
}

// fingerprint identifies a dataset by everything except what it is called.
//
// Two reports that ask the same question of the same source are one governed
// dataset, which is the whole reason the format separates them — importing
// forty copies of one query would carry the mistake across rather than the
// meaning. Identity is excluded so two identical queries under different report
// names still match, and nothing else is: two datasets that differ by one field
// label are two datasets, because a report bound to the wrong one shows the
// wrong headings.
func fingerprint(ds definition.Dataset) (string, error) {
	ds.Name, ds.Title, ds.Description = "x", "", ""
	raw, err := (codec.Encoder{}).Dataset(ds)
	return string(raw), err
}

// file writes one definition, refusing to clobber a different one.
func (im *importer) file(kind, name string, raw []byte) error {
	if im.out == "" {
		// Counted anyway: the question a dry run answers is how many
		// definitions this would produce, and reporting zero of them would make
		// the safe mode the useless one.
		im.wrote++
		return nil
	}
	path := filepath.Join(im.out, kind, name+".yaml")
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == string(raw) {
			// Re-running an import over its own output changes nothing, which
			// is what makes it safe to run twice.
			return nil
		}
		if !im.force {
			return fmt.Errorf("%s exists and differs — pass -force to overwrite it", path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return err
	}
	im.wrote++
	return nil
}

// report prints one file's findings.
//
// Only files that need a person are printed. An estate is four hundred files and
// nearly all of them lose nothing but fonts; a tool that prints a paragraph
// about each one produces a log nobody reads, and the blocked file in the middle
// of it is the one that mattered.
