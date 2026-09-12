//go:build !duckdb

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
What a pure-Go build says when a dataset needs federation.

Both of these used to hold in every build, because nothing imported the
federation package and the registry refused before attempting. Now the registry
asks for one and the answer depends on how the binary was built — so the
refusal is a property of this build, and belongs behind the tag that decides
it. The tagged half is in federate_test.go, and asserts that it works.
*/
func TestFederationSaysWhatItNeeds(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{
		source("warehouse", seed(t, "warehouse", "a", 1)),
		source("archive", seed(t, "archive", "b", 1)),
	}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	ds := dataset("joined", "warehouse")
	ds.Sources = append(ds.Sources, definition.SourceRef{Ref: "archive"})

	_, err = reg.Engine(context.Background(), ds)
	if !errors.Is(err, registry.ErrNoFederation) {
		t.Fatalf("got %v, want ErrNoFederation", err)
	}
	if !strings.Contains(err.Error(), "-tags duckdb") {
		t.Errorf("the message should say how to get it: %v", err)
	}
	// The sources it could not join, so the message is actionable without
	// going back to the definition to find out which dataset reads what.
	for _, want := range []string{"warehouse", "archive"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message should name %q: %v", want, err)
		}
	}
}

// An object store holds files rather than a catalogue, so reading one needs an
// engine that can address them even when it is the only source.
func TestAnObjectStoreAloneStillNeedsAnEngine(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{{
		Name: "lake", Driver: "object-store", URI: "s3://b/x", Format: "parquet",
	}}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	_, err = reg.Engine(context.Background(), dataset("events", "lake"))
	if !errors.Is(err, registry.ErrNoFederation) {
		t.Fatalf("got %v, want ErrNoFederation", err)
	}
	if !strings.Contains(err.Error(), "object store") {
		t.Errorf("the message should say why one source still needs it: %v", err)
	}
}
