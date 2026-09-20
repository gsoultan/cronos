package registry_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/driver/registry"
	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
Connecting a datasource while the process is running.

The registry opened what it was given at startup and nothing after, which was
the whole story while sources came from a directory. They come from the API
now, and a source published through it was a definition with no connection
behind it: the catalogue listed it, the test endpoint said "no such
datasource", and every report reading it failed until a restart.

These are the unit half. scripts/live-datasources.sh is the other, because the
failure was never in this package's logic — it was that nothing called it.
*/

// The claim, at its smallest: a source that was not there is there afterwards.
func TestAnAdoptedSourceAnswersADatasetThatNamesIt(t *testing.T) {
	reg, err := registry.New(nil, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	if _, err := reg.Engine(context.Background(), dataset("invoices", "warehouse")); !errors.Is(err, registry.ErrUnknownSource) {
		t.Fatalf("before adopting, got %v, want ErrUnknownSource", err)
	}

	if err := reg.Adopt(source("warehouse", seed(t, "warehouse", "from-warehouse", 1))); err != nil {
		t.Fatal(err)
	}

	if got := readID(t, reg, dataset("invoices", "warehouse")); got != "from-warehouse" {
		t.Errorf("read %q, want from-warehouse", got)
	}
	if names := reg.Names(); len(names) != 1 || names[0] != "warehouse" {
		t.Errorf("Names is %v — the test endpoint and the metrics read that", names)
	}
	if reg.Empty() {
		t.Error("a registry holding a source reports itself empty")
	}
}

/*
A second source does not disturb the first.

The case the whole change exists for: a project starts with one warehouse and
connects another, and the reports on the first must not notice. A registry that
rebuilt itself on every publish would pass the previous test and fail this one
by dropping the pool a query was already using.
*/
func TestAdoptingASecondSourceLeavesTheFirstAlone(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{
		source("warehouse", seed(t, "warehouse", "from-warehouse", 1)),
	}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	if err := reg.Adopt(source("archive", seed(t, "archive", "from-archive", 1))); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ dataset, from, want string }{
		{"invoices", "warehouse", "from-warehouse"},
		{"history", "archive", "from-archive"},
	} {
		if got := readID(t, reg, dataset(c.dataset, c.from)); got != c.want {
			t.Errorf("%s read %q from %s, want %q", c.dataset, got, c.from, c.want)
		}
	}
}

/*
Re-publishing an edited source replaces the connection rather than adding one.

Editing a datasource is the ordinary case — a rotated password, a failover
address — and it arrives under the name it already had. Keeping the old pool
would mean the edit silently did nothing, which is worse than failing: the
person watched the form save.
*/
func TestAdoptingTheSameNameReplacesWhatWasThere(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{
		source("warehouse", seed(t, "warehouse", "the-old-one", 1)),
	}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	if err := reg.Adopt(source("warehouse", seed(t, "moved", "the-new-one", 1))); err != nil {
		t.Fatal(err)
	}

	if got := readID(t, reg, dataset("invoices", "warehouse")); got != "the-new-one" {
		t.Errorf("read %q after re-publishing the source, want the-new-one", got)
	}
	if names := reg.Names(); len(names) != 1 {
		t.Errorf("replacing a source left %v — the same name is registered twice", names)
	}
}

/*
A source this build cannot open is refused, and changes nothing.

The same shape startup has: the caller decides whether that is fatal. What must
not happen is a half-registered source — a name in the map with no connection
behind it, which fails at the first query with the driver's words instead of
the source's name.
*/
func TestAdoptingASourceThisBuildCannotOpenChangesNothing(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{
		source("warehouse", seed(t, "warehouse", "from-warehouse", 1)),
	}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	err = reg.Adopt(definition.DataSource{Name: "lake", Driver: "no-such-driver", DSN: "x"})
	if err == nil {
		t.Fatal("a driver this build has no import for was adopted")
	}
	if !strings.Contains(err.Error(), "lake") {
		t.Errorf("the message should name the source: %v", err)
	}
	if names := reg.Names(); len(names) != 1 || names[0] != "warehouse" {
		t.Errorf("a refused source left the registry holding %v", names)
	}
	if got := readID(t, reg, dataset("invoices", "warehouse")); got != "from-warehouse" {
		t.Errorf("a refused adopt disturbed another source: read %q", got)
	}
}

// A source with no name would be registered under "", matched by no dataset
// and closed by nothing.
func TestAdoptingANamelessSourceIsRefused(t *testing.T) {
	reg, err := registry.New(nil, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	if err := reg.Adopt(definition.DataSource{Driver: "sqlite", DSN: ":memory:"}); err == nil {
		t.Fatal("a nameless source was adopted")
	}
	if !reg.Empty() {
		t.Errorf("a refused source is registered: %v", reg.Names())
	}
}

/*
Dropping one closes it, and the dataset that read it says so.

Deleting a definition used to leave the pool open and the source answering, so
the delete appeared to have done nothing until the next restart — at which
point a report that had worked all afternoon stopped, for a reason nobody was
still near.
*/
func TestADroppedSourceStopsAnsweringAndTakesTheOthersWithNothing(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{
		source("warehouse", seed(t, "warehouse", "from-warehouse", 1)),
		source("archive", seed(t, "archive", "from-archive", 1)),
	}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	reg.Drop("archive")

	_, err = reg.Engine(context.Background(), dataset("history", "archive"))
	if !errors.Is(err, registry.ErrUnknownSource) {
		t.Fatalf("a dropped source answered: %v", err)
	}
	if got := readID(t, reg, dataset("invoices", "warehouse")); got != "from-warehouse" {
		t.Errorf("dropping one source disturbed another: read %q", got)
	}
	if names := reg.Names(); len(names) != 1 || names[0] != "warehouse" {
		t.Errorf("after dropping, Names is %v", names)
	}
}

// Dropping a name nothing holds is what deleting a definition on a deployment
// that never opened it looks like, and it is not an error.
func TestDroppingSomethingThatWasNeverThereIsQuiet(t *testing.T) {
	reg, err := registry.New(nil, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	reg.Drop("nothing-here")
	if !reg.Empty() {
		t.Error("dropping an unknown name registered it")
	}
}

// Adopting into a closed registry would leak a pool to somebody else's
// warehouse with nothing left to release it.
func TestAdoptingAfterCloseIsRefused(t *testing.T) {
	reg, err := registry.New(nil, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}

	err = reg.Adopt(source("warehouse", seed(t, "warehouse", "x", 1)))
	if !errors.Is(err, registry.ErrClosed) {
		t.Fatalf("got %v, want ErrClosed", err)
	}
}
