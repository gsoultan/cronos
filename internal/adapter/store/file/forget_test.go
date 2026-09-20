package file

import (
	"context"
	"strings"
	"testing"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
)

/*
What the running view is told about a delete.

Apply put a published definition into it and nothing took one out, so a
deployment whose store is a database deleted a definition and went on serving
it: the report was gone from the store, gone from the next restart, and still
rendering. The person who deleted it saw a 204 and the report on their screen.
*/

const aSource = `
apiVersion: cronos.dev/v1
kind: DataSource
metadata:
  name: warehouse
spec:
  driver: sqlite
  dsn: "file:warehouse.db"
`

func TestForgettingADefinitionTakesItOutOfEveryAnswer(t *testing.T) {
	r := empty()
	if err := r.Apply([]byte(aSource)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DataSource(context.Background(), "warehouse"); err != nil {
		t.Fatalf("the fixture never landed: %v", err)
	}

	r.Forget(codec.KindDataSource, "warehouse")

	if _, err := r.DataSource(context.Background(), "warehouse"); err == nil {
		t.Error("a forgotten datasource still resolves")
	}
	if got := len(r.DataSources()); got != 0 {
		t.Errorf("it is still in the catalogue: %d sources", got)
	}
	// Names is what the management API lists, and raws is what it answers a
	// read with — a definition that is gone from one and present in the other
	// is a 200 for something that does not exist.
	if got := r.Names(codec.KindDataSource); len(got) != 0 {
		t.Errorf("it is still listed: %v", got)
	}
	if _, ok := r.Raw(codec.KindDataSource, "warehouse"); ok {
		t.Error("its document is still being served")
	}
}

// Forgetting what was never there is what deleting a definition on a
// deployment that reloads its own directory looks like, and it is quiet.
func TestForgettingSomethingThatIsNotThereIsQuiet(t *testing.T) {
	r := empty()
	r.Forget(codec.KindReport, "never-existed")
	r.Forget("NotAKind", "whatever")

	if got := len(r.Reports()); got != 0 {
		t.Errorf("forgetting an absent report left %d behind", got)
	}
}

// One name, one kind. Two definitions may share a name across kinds — a
// dataset and the report over it are commonly both "invoices" — so forgetting
// one must not take the other.
func TestForgettingOneKindLeavesTheOther(t *testing.T) {
	r := empty()
	if err := r.Apply([]byte(aSource)); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply([]byte(strings.ReplaceAll(goodReport, "billing-summary", "warehouse"))); err != nil {
		t.Fatal(err)
	}

	r.Forget(codec.KindDataSource, "warehouse")

	if got := len(r.Reports()); got != 1 {
		t.Errorf("forgetting the source took the report with it: %d reports", got)
	}
}
