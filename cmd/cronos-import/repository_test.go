package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/store/file"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// TestTheServerLoadsWhatTheImporterWrites is the end of the chain, and the only
// assertion that covers the whole of it.
//
// Every other test checks a step: the translation reads the band, the encoder
// writes the document, the loader reads it back. This one points the real
// repository — the thing cronosd reads at startup — at a directory the importer
// produced, and then compiles a block out of it against a principal. A
// migration that produces definitions the server will not serve has produced
// nothing, and this is what would notice.
func TestTheServerLoadsWhatTheImporterWrites(t *testing.T) {
	from, err := filepath.Abs("../../internal/adapter/codec/jrxml/testdata")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(from); err != nil {
		t.Skipf("fixtures not present: %v", err)
	}
	out := filepath.Join(t.TempDir(), "definitions")

	im := newImporter(out, true)
	// Three of the fixtures are deliberately unimportable; the run still has to
	// write the rest.
	if err := im.walk([]string{from}); err != nil {
		t.Fatalf("import: %v", err)
	}

	repo, err := file.Load(out)
	if err != nil {
		t.Fatalf("the repository refused what the importer wrote: %v", err)
	}
	datasets, reports, _, _ := repo.Counts()
	if datasets == 0 || reports == 0 {
		t.Fatalf("loaded %d datasets and %d reports", datasets, reports)
	}

	ctx := context.Background()
	pr := principal.Principal{OrgID: "acme", ProjectID: "finance"}
	compiled := 0
	for _, r := range repo.Reports() {
		ds, err := repo.Dataset(ctx, r.Dataset)
		if err != nil {
			t.Errorf("report %q reads dataset %q, which is not in the repository: %v",
				r.Name, r.Dataset, err)
			continue
		}
		// The check publish runs, then an actual compile of every block. This is
		// where a column that does not exist, or a measure with no aggregate,
		// stops being a definition that loads and becomes an error.
		if err := query.Check(ds); err != nil {
			t.Errorf("dataset %q does not compile: %v", ds.Name, err)
			continue
		}
		compiled += compileBlocks(t, r, ds, pr)
	}
	// A loop over nothing passes every assertion in it. This is the guard that
	// makes the test fail when the fixtures move rather than quietly proving
	// nothing.
	if compiled < 5 {
		t.Errorf("compiled only %d blocks; the fixtures are not being exercised", compiled)
	}
}

func compileBlocks(t *testing.T, r definition.Report, ds definition.Dataset,
	pr principal.Principal) (compiled int) {
	t.Helper()
	// A real dialect and a fixed clock, because a zero Builder has neither and a
	// dataset defaulting to "today" needs one.
	b := query.NewBuilder(query.Postgres{}).WithClock(func() time.Time {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	})
	args := defaultArgs(ds)
	for _, out := range r.Outputs {
		for i, blk := range out.Layout {
			if blk.Kind == definition.TextBlock {
				continue
			}
			if _, _, err := b.BuildBlock(ds, blk, args, query.Filters{}, pr); err != nil {
				t.Errorf("%s/%s block %d (%s) will not compile: %v",
					r.Name, out.Name, i, blk.Kind, err)
				continue
			}
			compiled++
		}
	}
	return compiled
}

// defaultArgs supplies whatever the dataset requires, so compilation fails on
// the block rather than on a missing parameter.
func defaultArgs(ds definition.Dataset) map[string]any {
	args := map[string]any{}
	for _, p := range ds.Params {
		if !p.Required {
			continue
		}
		switch p.Type {
		case definition.Date:
			args[p.Name] = "2026-01-01"
		case definition.Number:
			args[p.Name] = 1
		case definition.Bool:
			args[p.Name] = true
		default:
			if p.Multiple {
				args[p.Name] = []any{"x"}
			} else {
				args[p.Name] = "x"
			}
		}
	}
	return args
}
