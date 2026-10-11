package definition

import (
	"errors"
	"strings"
	"testing"
)

func TestTemplateValidate(t *testing.T) {
	ok := Template{Name: "letterhead", Source: "#let header(ctx) = [Acme]"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a plain letterhead: %v", err)
	}
	for name, bad := range map[string]Template{
		"no name":   {Source: ok.Source},
		"no source": {Name: "letterhead"},
		"a package": {Name: "letterhead", Source: `#import "@preview/cetz:0.3.0": canvas`},
		"too large": {Name: "letterhead", Source: strings.Repeat("x", MaxTemplateSource+1)},
	} {
		if err := bad.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestTemplateDefines(t *testing.T) {
	tpl := Template{Source: "#let header(ctx) = [x]\n#let footer (ctx) = [y]\n#let headers(c) = 1"}
	if !tpl.Defines("header") || !tpl.Defines("footer") {
		t.Error("both are defined")
	}
	if (Template{Source: "#let headers(c) = 1"}).Defines("header") {
		t.Error("headers is not header")
	}
}
