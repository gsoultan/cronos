package run

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/document"
)

// Printable turns a view's chart blocks into marks a typesetter can place.
//
// Exported for the tests that assert what reaches paper. The decisions here
// are invisible until somebody opens a PDF, which is the worst place to find
// out that a pie was drawn as an ellipse.
//
// The arithmetic happens here, once, rather than in the template — see
// document.Mark. A typesetter is not a charting library, and the alternative
// was fourteen drawing routines in Typst kept in step with fourteen more in
// the browser.
func Printable(view View) []document.Chart {
	var out []document.Chart
	for _, b := range view.Blocks {
		if b.Kind != "chart" {
			continue
		}
		out = append(out, chartOf(b))
	}
	return out
}

func chartOf(b Block) document.Chart {
	// Marks starts empty rather than nil: a nil slice marshals as null, which
	// the template reads as `none` and cannot call `.len()` on — so a chart
	// that carries a note instead of marks took the whole document down.
	c := document.Chart{Title: b.Title, Kind: b.Chart, Marks: []document.Mark{}}
	switch {
	case b.Map != nil:
		// The geometry reaches the browser as SVG path strings, which a
		// typesetter cannot read — it wants vertices. Saying so beats a blank
		// space where a map was asked for.
		c.Note = "Maps are not printed. Open this report in a browser to see it."
	case b.Gauge != nil:
		gauge(&c, b.Gauge)
		c.Square = true
	case b.Rects != nil:
		treemap(&c, b.Rects)
	case b.Stages != nil:
		funnel(&c, b.Stages)
	case b.Steps != nil:
		waterfall(&c, b.Steps, b.YAxis)
	case b.Cells != nil:
		heatmap(&c, b)
	case b.Points != nil:
		plot(&c, b)
	case b.Tracks != nil:
		combo(&c, b)
	case b.Groups != nil:
		grouped(&c, b)
	case b.Chart == "pie" || b.Chart == "donut":
		pie(&c, b.Series, b.Chart == "donut")
		c.Square = true
	case b.Chart == "line" || b.Chart == "area":
		line(&c, b)
	default:
		bars(&c, b.Series)
	}
	if len(c.Marks) == 0 && c.Note == "" {
		c.Note = "No data in this period."
	}
	return c
}

// bars draws the horizontal bars a bar chart is, which is the same arrangement
// the browser uses — a reader holding both should not have to translate.
func bars(c *document.Chart, series []Bar) {
	max := 0.0
	for _, s := range series {
		max = maxOf(max, s.Value)
	}
	slot := 1.0 / float64(maxInt(len(series), 1))
	for i, s := range series {
		c.Marks = append(c.Marks, document.Mark{
			Kind: document.RectMark,
			X:    0, Y: float64(i)*slot + slot*0.15,
			W: share(s.Value, max), H: slot * 0.7,
			Tone: "series-1", Label: s.Label, Value: s.Formatted,
		})
	}
}

// grouped draws one lane per series inside each bucket, or one stack when the
// block asked to be stacked.
func grouped(c *document.Chart, b Block) {
	buckets := 0
	if len(b.Groups) > 0 {
		buckets = len(b.Groups[0].Bars)
	}
	max := 0.0
	for i := range buckets {
		total := 0.0
		for _, g := range b.Groups {
			if b.Stacked {
				total += g.Bars[i].Value
			} else {
				max = maxOf(max, g.Bars[i].Value)
			}
		}
		max = maxOf(max, total)
	}

	slot := 1.0 / float64(maxInt(buckets, 1))
	for i := range buckets {
		at := 0.0
		for j, g := range b.Groups {
			// A series with nothing in this bucket contributes no mark. The
			// floor in share() would otherwise draw it as a sliver, and a
			// sliver reads as a small amount rather than as none — which is
			// the opposite of what it is, and disagrees with the viewer.
			if g.Bars[i].Value == 0 {
				continue
			}
			w := share(g.Bars[i].Value, max)
			m := document.Mark{
				Kind: document.RectMark, Tone: tone(g.Slot),
				Label: g.Bars[i].Label, Value: g.Bars[i].Formatted,
			}
			if b.Stacked {
				m.X, m.Y, m.W, m.H = at, float64(i)*slot+slot*0.15, w, slot*0.7
				at += w
			} else {
				lane := slot * 0.7 / float64(len(b.Groups))
				m.X, m.Y, m.W, m.H = 0, float64(i)*slot+slot*0.15+float64(j)*lane, w, lane*0.85
			}
			c.Marks = append(c.Marks, m)
		}
	}
	c.Keys = keysOf(b.Groups)
}

// line draws a polyline across the buckets, filled underneath for an area.
func line(c *document.Chart, b Block) {
	groups := b.Groups
	if len(groups) == 0 {
		groups = []Group{{Label: b.Title, Bars: b.Series}}
	}
	lo, hi := scaleOf(b.YAxis, groups)

	for _, g := range groups {
		pts := make([][2]float64, 0, len(g.Bars))
		for i, bar := range g.Bars {
			pts = append(pts, [2]float64{at(i, len(g.Bars)), 1 - norm(bar.Value, lo, hi)})
		}
		if len(pts) < 2 {
			continue
		}
		if b.Chart == "area" {
			// Closed back along the baseline, which is what makes it an area
			// rather than a line with a stray edge.
			poly := append(append([][2]float64{}, pts...),
				[2]float64{pts[len(pts)-1][0], 1}, [2]float64{pts[0][0], 1})
			c.Marks = append(c.Marks, document.Mark{
				Kind: document.PolyMark, Points: poly, Tone: tone(g.Slot) + "-wash",
			})
		}
		c.Marks = append(c.Marks, document.Mark{
			Kind: document.LineMark, Points: pts, Tone: tone(g.Slot), Label: g.Label,
		})
	}
	c.Ticks = ticksOf(b.YAxis)
	if len(groups) > 1 {
		c.Keys = keysOf(groups)
	}
}

// combo draws each measure the way it asked to be drawn.
func combo(c *document.Chart, b Block) {
	buckets := 0
	if len(b.Tracks) > 0 {
		buckets = len(b.Tracks[0].Bars)
	}
	slot := 1.0 / float64(maxInt(buckets, 1))

	barTracks := 0
	for _, t := range b.Tracks {
		if t.Draw == "bar" {
			barTracks++
		}
	}

	drawn := 0
	for _, t := range b.Tracks {
		lo, hi := scaleOf(axisFor(b, t), []Group{{Bars: t.Bars}})
		if t.Draw == "line" {
			pts := make([][2]float64, 0, len(t.Bars))
			for i, bar := range t.Bars {
				pts = append(pts, [2]float64{at(i, len(t.Bars)), 1 - norm(bar.Value, lo, hi)})
			}
			if len(pts) >= 2 {
				c.Marks = append(c.Marks, document.Mark{
					Kind: document.LineMark, Points: pts, Tone: tone(t.Slot), Label: t.Label,
				})
			}
			continue
		}
		for i, bar := range t.Bars {
			h := norm(bar.Value, lo, hi)
			lane := slot * 0.8 / float64(maxInt(barTracks, 1))
			c.Marks = append(c.Marks, document.Mark{
				Kind: document.RectMark,
				X:    float64(i)*slot + slot*0.1 + float64(drawn)*lane,
				Y:    1 - h, W: lane * 0.9, H: h,
				Tone: tone(t.Slot), Label: bar.Label, Value: bar.Formatted,
			})
		}
		drawn++
	}
	c.Ticks = ticksOf(b.YAxis)
	c.Keys = trackKeys(b.Tracks)
}

// axisFor is the scale a track reads against — its own when it asked to leave
// the shared one.
func axisFor(b Block, t Track) *Axis {
	if t.Secondary && b.Axis2 != nil {
		return b.Axis2
	}
	return b.YAxis
}

// waterfall draws each step floating between two running totals.
func waterfall(c *document.Chart, steps []Step, y *Axis) {
	lo, hi := 0.0, 0.0
	for _, s := range steps {
		lo, hi = minOf(lo, minOf(s.Start, s.End)), maxOf(hi, maxOf(s.Start, s.End))
	}
	if y != nil {
		lo, hi = y.Min, y.Max
	}
	slot := 1.0 / float64(maxInt(len(steps), 1))

	for i, s := range steps {
		top := 1 - norm(maxOf(s.Start, s.End), lo, hi)
		bottom := 1 - norm(minOf(s.Start, s.End), lo, hi)
		c.Marks = append(c.Marks, document.Mark{
			Kind: document.RectMark,
			X:    float64(i)*slot + slot*0.15, Y: top,
			W: slot * 0.7, H: maxOf(bottom-top, 0.004),
			Tone: sign(s), Label: s.Label, Value: s.Formatted,
		})
	}
	c.Ticks = ticksOf(y)
}

func sign(s Step) string {
	switch {
	case s.Total:
		return "neutral"
	case s.Sign < 0:
		return "down"
	}
	return "up"
}

// funnel draws centred bands, narrowing down the page.
func funnel(c *document.Chart, stages []Stage) {
	slot := 1.0 / float64(maxInt(len(stages), 1))
	for i, s := range stages {
		w := maxOf(s.Share, 0.04)
		c.Marks = append(c.Marks, document.Mark{
			Kind: document.RectMark,
			X:    (1 - w) / 2, Y: float64(i)*slot + slot*0.2,
			W: w, H: slot * 0.55,
			Tone: step(i), Label: s.Label, Value: s.Formatted,
		})
	}
}

// heatmap draws the grid, which is already dense.
func heatmap(c *document.Chart, b Block) {
	cols, rows := len(b.HeatColumns), len(b.HeatRows)
	if cols == 0 || rows == 0 {
		return
	}
	w, h := 1/float64(cols), 1/float64(rows)
	for _, cell := range b.Cells {
		x, y := indexOf(b.HeatColumns, cell.Column), indexOf(b.HeatRows, cell.Row)
		if x < 0 || y < 0 || cell.Empty {
			continue
		}
		c.Marks = append(c.Marks, document.Mark{
			Kind: document.RectMark,
			X:    float64(x) * w, Y: float64(y) * h, W: w * 0.94, H: h * 0.9,
			Tone:  fmt.Sprintf("ramp-%d", minInt(cell.Step, 5)+1),
			Label: cell.Row + " · " + cell.Column, Value: cell.Formatted,
		})
	}
}

// treemap places the rectangles the server already laid out.
func treemap(c *document.Chart, rects []Rect) {
	for _, r := range rects {
		if r.Depth == 0 && len(rects) > 1 && hasLeaves(rects) {
			// The group's frame is drawn by its leaves; filling it would hide
			// them.
			continue
		}
		c.Marks = append(c.Marks, document.Mark{
			Kind: document.RectMark,
			X:    r.X, Y: r.Y, W: r.W * 0.99, H: r.H * 0.99,
			Tone: toneFor(r), Label: r.Label, Value: r.Formatted,
		})
	}
}

func hasLeaves(rects []Rect) bool {
	for _, r := range rects {
		if r.Depth == 1 {
			return true
		}
	}
	return false
}

func toneFor(r Rect) string {
	if r.Depth == 1 {
		return tone(r.Slot)
	}
	return step(r.Slot)
}

// plot draws the dots of a scatter or a bubble.
func plot(c *document.Chart, b Block) {
	if b.XAxis == nil || b.YAxis == nil {
		return
	}
	for _, p := range b.Points {
		r := 0.012
		if p.Weight > 0 {
			r = 0.014 + sqrt(p.Weight)*0.035
		}
		c.Marks = append(c.Marks, document.Mark{
			Kind: document.DotMark,
			X:    norm(p.X, b.XAxis.Min, b.XAxis.Max),
			Y:    1 - norm(p.Y, b.YAxis.Min, b.YAxis.Max),
			W:    r, Tone: tone(p.Slot), Label: p.Label, Value: p.FY,
		})
	}
	c.Ticks = ticksOf(b.YAxis)
}

// pie draws each slice as a polygon.
//
// A polygon and not an arc: Typst has no arc primitive, and a slice at this
// size is indistinguishable from one drawn with enough vertices. Computing
// them here keeps the trigonometry out of the template.
func pie(c *document.Chart, series []Bar, donut bool) {
	total := 0.0
	for _, s := range series {
		if s.Value > 0 {
			total += s.Value
		}
	}
	if total <= 0 {
		return
	}
	inner := 0.0
	if donut {
		inner = 0.55
	}

	from := -0.25 // Twelve o'clock, in turns.
	for i, s := range series {
		if s.Value <= 0 {
			continue
		}
		span := s.Value / total
		c.Marks = append(c.Marks, document.Mark{
			Kind: document.PolyMark, Points: slice(from, span, inner),
			Tone: tone(i), Label: s.Label, Value: s.Formatted,
		})
		from += span
	}
	c.Keys = sliceKeys(series)
}

// gauge draws the track and the arc over it, as the same polygons a pie uses.
func gauge(c *document.Chart, g *Gauge) {
	const sweep = 240.0 / 360
	from := -0.5 - sweep/2
	c.Marks = append(c.Marks,
		document.Mark{Kind: document.PolyMark, Points: slice(from, sweep, 0.68), Tone: "line"},
		document.Mark{
			Kind: document.PolyMark, Points: slice(from, sweep*g.Share, 0.68),
			Tone: "series-1", Label: g.Formatted,
			Value: g.TargetLabel + " " + g.TargetFormatted,
		})
	if g.Over != "" {
		c.Note = g.Formatted + " · " + g.Over
	}
}
