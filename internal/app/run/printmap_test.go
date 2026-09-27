package run

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/document"
)

// square is a closed subpath in world units, as geoPath writes one.
func square(x0, y0, x1, y1 float64) string {
	return fmt.Sprintf("M%g %gL%g %gL%g %gL%g %gZ", x0, y0, x1, y0, x1, y1, x0, y1)
}

// painted is the order paper sees the areas in: each mark's label, or
// "paper" for a hole cut back out.
func painted(c document.Chart) string {
	var out []string
	for _, m := range c.Marks {
		if m.Tone == "paper" {
			out = append(out, "paper")
			continue
		}
		out = append(out, m.Label)
	}
	return strings.Join(out, " ")
}

// A lake inside a region is cut out in paper white after the region; an
// island in that lake has to be painted after the lake, or the lake paints it
// out. Each shape's holes went after all of its fills, and the island went
// with them.
func TestAnIslandInALakeIsStillOnPaper(t *testing.T) {
	var c document.Chart
	printMap(&c, &GeoMap{
		Bounds: Bounds{MaxX: 1, MaxY: 1},
		Layers: []string{"polygon"},
		Shapes: []Shape{{
			Label: "Region",
			Path:  square(0.1, 0.1, 0.9, 0.9) + square(0.3, 0.3, 0.7, 0.7) + square(0.45, 0.45, 0.55, 0.55),
		}},
	})
	if got := painted(c); got != "Region paper Region" {
		t.Errorf("painted %q, want the region, its lake, then the island in the lake", got)
	}
}

// An exclave of one region inside a hole of another is painted after that
// hole. Shapes went largest first, so an exclave belonging to a region that
// is large elsewhere was painted first — and whited out by the hole around it.
func TestAnExclaveInAnotherRegionsHoleIsStillOnPaper(t *testing.T) {
	var c document.Chart
	printMap(&c, &GeoMap{
		Bounds: Bounds{MaxX: 1, MaxY: 1},
		Layers: []string{"polygon"},
		Shapes: []Shape{
			{Label: "Host", Path: square(0.1, 0.1, 0.5, 0.5) + square(0.2, 0.2, 0.4, 0.4)},
			// Its main part far away makes its box the larger of the two.
			{Label: "Exclave", Path: square(0.25, 0.25, 0.35, 0.35) + square(0.6, 0.05, 0.95, 0.95)},
		},
	})
	got := painted(c)
	hole := strings.Index(got, "paper")
	last := strings.LastIndex(got, "Exclave")
	if hole < 0 || last < hole {
		t.Errorf("painted %q, want the exclave's piece after the hole it sits in", got)
	}
}
