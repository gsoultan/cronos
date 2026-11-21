package explain

import (
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/app/run"
)

// chart writes whichever of a chart's collections it carries. Most carry one;
// the rest are empty and write nothing.
func chart(b *strings.Builder, blk run.Block) {
	bars(b, "Series", blk.Series)
	for _, g := range blk.Groups {
		bars(b, "Series "+clip(g.Label), g.Bars)
	}
	for _, t := range blk.Tracks {
		bars(b, "Measure "+clip(t.Label), t.Bars)
	}
	if g := blk.Gauge; g != nil {
		line(b, "Value: %s against a target of %s (%s)", g.Formatted, g.TargetFormatted, g.TargetLabel)
		if g.Over != "" {
			line(b, "Over target by %s", g.Over)
		}
	}
	points(b, "Stages", len(blk.Stages), func(i int) string {
		return blk.Stages[i].Label + ": " + blk.Stages[i].Formatted
	})
	points(b, "Steps", len(blk.Steps), func(i int) string {
		return blk.Steps[i].Label + ": " + blk.Steps[i].Formatted
	})
	points(b, "Cells", len(blk.Cells), func(i int) string {
		c := blk.Cells[i]
		return c.Row + " / " + c.Column + ": " + c.Formatted
	})
	points(b, "Parts", len(blk.Rects), func(i int) string {
		return blk.Rects[i].Label + ": " + blk.Rects[i].Formatted
	})
	points(b, "Rows", len(blk.Bullets), func(i int) string {
		u := blk.Bullets[i]
		return u.Label + ": " + u.Formatted + " against " + u.TargetFormatted
	})
	points(b, "Bins", len(blk.Bins), func(i int) string {
		return blk.Bins[i].Label + ": " + blk.Bins[i].Formatted
	})
	points(b, "Points", len(blk.Points), func(i int) string {
		p := blk.Points[i]
		return fmt.Sprintf("%s (%s, %s)", p.Label, p.FX, p.FY)
	})
	points(b, "Days", len(blk.Days), func(i int) string {
		return blk.Days[i].Date + ": " + blk.Days[i].Formatted
	})
}

func bars(b *strings.Builder, name string, bs []run.Bar) {
	points(b, name, len(bs), func(i int) string {
		return bs[i].Label + ": " + bs[i].Formatted
	})
}

// points writes a list as one line, keeping the first and last when it is
// longer than mostPoints.
func points(b *strings.Builder, name string, n int, at func(int) string) {
	if n == 0 {
		return
	}
	half := mostPoints / 2
	parts := make([]string, 0, min(n, mostPoints+1))
	for i := range n {
		if n > mostPoints && i == half {
			parts = append(parts, fmt.Sprintf("… %d more …", n-mostPoints))
		}
		if n > mostPoints && i >= half && i < n-half {
			continue
		}
		parts = append(parts, clip(at(i)))
	}
	line(b, "%s: %s", name, strings.Join(parts, "; "))
}
