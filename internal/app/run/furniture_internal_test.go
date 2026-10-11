package run

import (
	"context"
	"errors"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

type oneTemplate struct{ t definition.Template }

func (o oneTemplate) Template(_ context.Context, name string) (definition.Template, error) {
	if name != o.t.Name {
		return definition.Template{}, errors.New("no such template")
	}
	return o.t, nil
}

// The counters are spelt however an author spelt them and reach the template
// in the one spelling it replaces.
func TestFurnitureTextCarriesTheCounters(t *testing.T) {
	got, err := (&Statements{}).furniture(context.Background(),
		definition.Furniture{Text: "Page {{ .page }} of {{.pages}} · {{ .other }}"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "Page {{page}} of {{pages}} · {{ .other }}"; got.Text != want {
		t.Errorf("got %q, want %q", got.Text, want)
	}
}

func TestFurnitureTemplateCarriesItsSource(t *testing.T) {
	s := (&Statements{}).WithTemplates(oneTemplate{definition.Template{
		Name: "letterhead", Source: "#let header(ctx) = []"}})
	got, err := s.furniture(context.Background(), definition.Furniture{Template: "letterhead"})
	if err != nil || got.Template != "#let header(ctx) = []" {
		t.Fatalf("got %+v, %v", got, err)
	}
	// Gone since the report was published: the render fails rather than
	// printing a statement without the letterhead it was designed with.
	if _, err := s.furniture(context.Background(),
		definition.Furniture{Template: "gone"}); !errors.Is(err, ErrNotRenderable) {
		t.Fatalf("a missing template rendered: %v", err)
	}
}
