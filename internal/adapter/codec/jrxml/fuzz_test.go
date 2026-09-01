package jrxml

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzImport checks that no input makes the importer panic.
//
// The realistic failure mode for a migration tool is not a wrong answer, it is
// file 287 of 400 taking the process down — and a `.jrxml` estate is full of
// files nobody has opened in a decade: truncated by a failed copy, half-merged
// by a version control conflict, written by a Studio build that no longer
// exists. A crash there costs the whole run and tells the operator nothing
// about which file did it.
//
// So the contract is narrow and total: Import returns a Result and an error,
// for every possible input. It may refuse anything; it may not panic.
func FuzzImport(f *testing.F) {
	// Seeded with the real fixtures, so the corpus starts from documents that
	// exercise the translation rather than from strings that fail at the first
	// byte.
	seeds, _ := filepath.Glob("testdata/*.jrxml")
	for _, path := range seeds {
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
	}
	f.Add([]byte(`<jasperReport name="x"><queryString>SELECT 1</queryString></jasperReport>`))
	f.Add([]byte(""))

	imp := Importer{DataSource: "warehouse", Folder: "/imported"}
	f.Fuzz(func(t *testing.T, data []byte) {
		res, err := imp.Import(data)
		if err != nil {
			// A refusal still has to be able to explain itself.
			_ = res.Findings
			return
		}
		// Anything that imported has to be something the engine would store:
		// emitting a definition that does not validate would put a broken file
		// in a definitions directory and blame it on the author.
		if res.HasDataset() {
			if err := res.Dataset.Validate(); err != nil {
				t.Fatalf("imported an invalid dataset: %v\n%q", err, data)
			}
		}
		if res.HasReport() {
			if err := res.Report.Validate(); err != nil {
				t.Fatalf("imported an invalid report: %v\n%q", err, data)
			}
			if res.Report.Dataset == "" {
				t.Fatalf("imported a report bound to no dataset\n%q", data)
			}
		}
	})
}
