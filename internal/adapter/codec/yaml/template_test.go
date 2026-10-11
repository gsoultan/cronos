package yaml_test

import (
	"strings"
	"testing"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/core/definition"
)

// A template is written as a file somebody reads and diffs: the source as a
// literal block, and back in as the same string.
func TestATemplateRoundTrips(t *testing.T) {
	in := definition.Template{Name: "letterhead", Title: "Letterhead",
		Source: "#let header(ctx) = [\n  *Acme* #h(1fr) #ctx.group\n]\n"}
	raw, err := codec.Encoder{}.Template(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "source: |") {
		t.Errorf("the source is not a literal block:\n%s", raw)
	}
	kind, err := codec.Loader{}.Kind(raw)
	if err != nil || kind != codec.KindTemplate {
		t.Fatalf("kind %q, %v", kind, err)
	}
	out, err := codec.Loader{}.Template(raw)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("came back as %+v", out)
	}
}
