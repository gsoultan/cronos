package run

import (
	"fmt"
	"slices"
	"strings"

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
// the browser. The words a chart carries — a bar's value, a category's name, a
// donut's whole — are placed here too, as text marks, so the page says what
// the screen says in the same places.
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
		printMap(&c, b.Map)
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
	case b.Chart == "line" || b.Chart == "area":
		line(&c, b)
	case b.Groups != nil:
		grouped(&c, b)
	case b.Chart == "pie" || b.Chart == "donut":
		pie(&c, b)
		c.Square = true
	default:
		bars(&c, b.Series)
	}
	// A word with nothing to say — a category with no name, a figure with no
	// formatting — is left off rather than sent: the document refuses a text
	// mark it cannot draw, and a blank name is not worth a failed burst.
	c.Marks = slices.DeleteFunc(c.Marks, func(m document.Mark) bool {
		return m.Kind == document.TextMark && strings.TrimSpace(m.Label) == ""
	})
	if len(c.Marks) == 0 && c.Note == "" {
		c.Note = "No data in this period."
	}
	return c
}

// bars draws the horizontal bars a bar chart is, which is the same arrangement
// the browser uses — a reader holding both should not have to translate: each
// category's name in a column on the left, its bar, and its value at the end.
func bars(c *document.Chart, series []Bar) {
	values := make([]float64, 0, len(series))
	for _, s := range series {
		values = append(values, s.Value)
	}
	x := barScale(values, nameColumn(labelsOf(series)))
	slot := 1.0 / float64(maxInt(len(series), 1))
	c.Height = rowsHeight(len(series), 7)
	for i, s := range series {
		y, h := float64(i)*slot+slot*0.2, slot*0.6
		c.Marks = append(c.Marks, barMark(x, s.Value, y, h, "series-1"),
			name(s.Label, y+h/2), valueAt(x, s.Value, s.Formatted, y+h/2))
	}
	zeroLine(c, x, values)
}

// grouped draws one lane per series inside each bucket, or one stack when the
// block asked to be stacked, each bucket named on the left.
func grouped(c *document.Chart, b Block) {
	if len(b.Groups) == 0 {
		return
	}
	buckets := len(b.Groups[0].Bars)
	x := barScale(groupEnds(b), nameColumn(labelsOf(b.Groups[0].Bars)))
	slot := 1.0 / float64(maxInt(buckets, 1))
	each := 7.0
	if !b.Stacked {
		each = 3.5*float64(len(b.Groups)) + 3
	}
	c.Height = rowsHeight(buckets, each)
	for i := range buckets {
		top := float64(i)*slot + slot*0.15
		c.Marks = append(c.Marks, name(b.Groups[0].Bars[i].Label, top+slot*0.35))
		if b.Stacked {
			stackRow(c, b, x, i, top, slot*0.7)
			continue
		}
		lane := slot * 0.7 / float64(len(b.Groups))
		for j, g := range b.Groups {
			// A series with nothing in this bucket contributes no mark. The
			// floor would otherwise draw it as a sliver, and a sliver reads
			// as a small amount rather than as none.
			if v := g.Bars[i]; v.Value != 0 {
				y := top + float64(j)*lane
				c.Marks = append(c.Marks, barMark(x, v.Value, y, lane*0.85, tone(g.Slot)))
			}
		}
	}
	c.Keys = keysOf(b.Groups)
}

// stackRow lays one bucket's parts end to end outward from nothing, and writes
// the total past the end it reaches on the total's side.
func stackRow(c *document.Chart, b Block, x func(float64) float64, i int, top, h float64) {
	up, down := 0.0, 0.0
	for _, g := range b.Groups {
		v := g.Bars[i].Value
		if v == 0 {
			continue
		}
		from := up
		if v < 0 {
			from = down + v
			down += v
		} else {
			up += v
		}
		c.Marks = append(c.Marks, document.Mark{Kind: document.RectMark,
			X: x(from), Y: top, W: maxOf(x(from+abs(v))-x(from), 0.003), H: h, Tone: tone(g.Slot)})
	}
	if i < len(b.Totals) {
		t := b.Totals[i]
		end := up
		if t.Value < 0 {
			end = down
		}
		c.Marks = append(c.Marks, valueAt(x, end, t.Formatted, top+h/2))
	}
}

// line draws a polyline through the middle of each bucket — where a column
// would stand, so a combo's line and its bars agree and the last label has
// the room the first has — filled underneath for an area.
func line(c *document.Chart, b Block) {
	groups := b.Groups
	if len(groups) == 0 {
		groups = []Group{{Label: b.Title, Bars: b.Series}}
	}
	lo, hi := scaleOf(b.YAxis, groups)
	floor := 1 - norm(maxOf(lo, 0), lo, hi)
	for _, g := range groups {
		pts := make([][2]float64, 0, len(g.Bars))
		for i, bar := range g.Bars {
			pts = append(pts, [2]float64{middle(i, len(g.Bars)), 1 - norm(bar.Value, lo, hi)})
		}
		if len(pts) < 2 {
			continue
		}
		if b.Chart == "area" {
			// Closed back along nothing, which is what makes it an area rather
			// than a line with a stray edge.
			poly := append(append([][2]float64{}, pts...),
				[2]float64{pts[len(pts)-1][0], floor}, [2]float64{pts[0][0], floor})
			c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark, Points: poly, Tone: tone(g.Slot) + "-wash"})
		}
		c.Marks = append(c.Marks, document.Mark{Kind: document.LineMark, Points: pts, Tone: tone(g.Slot), Stroke: 1.4})
		dots(c, pts, tone(g.Slot))
	}
	c.Ticks = ticksOf(b.YAxis)
	if len(groups) > 0 {
		c.XTicks = categories(labelsOf(groups[0].Bars))
	}
	if len(groups) > 1 {
		c.Keys = keysOf(groups)
	}
}

// combo draws each measure the way it asked to be drawn: bars side by side in
// each bucket and lines through their middles, a measure on its own scale read
// against a second one down the right in its colour.
func combo(c *document.Chart, b Block) {
	buckets := 0
	if len(b.Tracks) > 0 {
		buckets = len(b.Tracks[0].Bars)
	}
	slot := 1.0 / float64(maxInt(buckets, 1))
	bars := 0
	for _, t := range b.Tracks {
		if t.Draw == "bar" {
			bars++
		}
	}
	drawn := 0
	for _, t := range b.Tracks {
		lo, hi := scaleOf(axisFor(b, t), []Group{{Bars: t.Bars}})
		if t.Draw == "line" {
			comboLine(c, t, lo, hi)
			continue
		}
		lane := minOf(slot*0.72/float64(maxInt(bars, 1)), 0.12)
		for i, bar := range t.Bars {
			h := norm(bar.Value, lo, hi)
			left := float64(i)*slot + (slot-lane*float64(bars))/2 + float64(drawn)*lane
			c.Marks = append(c.Marks, document.Mark{Kind: document.RectMark,
				X: left, Y: 1 - h, W: lane * 0.9, H: h, Tone: tone(t.Slot)})
		}
		drawn++
	}
	c.Ticks, c.Keys = ticksOf(b.YAxis), trackKeys(b.Tracks)
	if buckets > 0 {
		c.XTicks = categories(labelsOf(b.Tracks[0].Bars))
	}
	secondScale(c, b)
}

func comboLine(c *document.Chart, t Track, lo, hi float64) {
	pts := make([][2]float64, 0, len(t.Bars))
	for i, bar := range t.Bars {
		pts = append(pts, [2]float64{middle(i, len(t.Bars)), 1 - norm(bar.Value, lo, hi)})
	}
	if len(pts) >= 2 {
		c.Marks = append(c.Marks, document.Mark{Kind: document.LineMark, Points: pts, Tone: tone(t.Slot), Stroke: 1.4})
	}
	dots(c, pts, tone(t.Slot))
}

// dots marks each point of a line, as the screen does while there are few
// enough to tell apart: a line through two or three values says where they
// are only at its bends.
func dots(c *document.Chart, pts [][2]float64, colour string) {
	if len(pts) > 24 {
		return
	}
	for _, p := range pts {
		c.Marks = append(c.Marks, document.Mark{Kind: document.DotMark, X: p[0], Y: p[1], W: pointRadius, Tone: colour})
	}
}

// secondScale puts a combo's second scale down the right, in the colour of
// the measure that asked for it.
func secondScale(c *document.Chart, b Block) {
	if b.Axis2 == nil {
		return
	}
	c.Ticks2 = ticksOf(b.Axis2)
	for _, t := range b.Tracks {
		if t.Secondary {
			c.Tone2 = tone(t.Slot)
		}
	}
}

// axisFor is the scale a track reads against — its own when it asked to leave
// the shared one.
func axisFor(b Block, t Track) *Axis {
	if t.Secondary && b.Axis2 != nil {
		return b.Axis2
	}
	return b.YAxis
}

// waterfall draws each step floating between two running totals, its change
// written over it, and a hairline carrying the running total to the next.
func waterfall(c *document.Chart, steps []Step, y *Axis) {
	lo, hi := 0.0, 0.0
	for _, s := range steps {
		lo, hi = minOf(lo, minOf(s.Start, s.End)), maxOf(hi, maxOf(s.Start, s.End))
	}
	if y != nil {
		lo, hi = y.Min, y.Max
	}
	slot := 1.0 / float64(maxInt(len(steps), 1))
	labels := make([]string, 0, len(steps))
	for i, s := range steps {
		top := 1 - norm(maxOf(s.Start, s.End), lo, hi)
		bottom := 1 - norm(minOf(s.Start, s.End), lo, hi)
		// Capped, as a column is on screen: three steps across a page were
		// columns a third of it wide.
		w := minOf(slot*0.64, 0.09)
		left := middle(i, len(steps)) - w/2
		c.Marks = append(c.Marks, document.Mark{Kind: document.RectMark,
			X: left, Y: top, W: w, H: maxOf(bottom-top, 0.004), Tone: sign(s)})
		at := top - 0.04
		if s.Sign < 0 && !s.Total {
			at = bottom + 0.05
		}
		c.Marks = append(c.Marks, text(left+w/2, at, s.Formatted, "ink", "", true))
		if i < len(steps)-1 {
			end := 1 - norm(s.End, lo, hi)
			c.Marks = append(c.Marks, document.Mark{Kind: document.LineMark, Tone: "neutral", Stroke: 0.4,
				Points: [][2]float64{{left + w, end}, {middle(i+1, len(steps)) - w/2, end}}})
		}
		labels = append(labels, s.Label)
	}
	c.Ticks, c.XTicks = ticksOf(y), categories(labels)
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

// funnel draws centred bands, narrowing down the page and joined by the neck
// each narrowing leaves, each stage named on the left and its value and share
// of the first on the right.
func funnel(c *document.Chart, stages []Stage) {
	labels := make([]string, len(stages))
	for i, s := range stages {
		labels[i] = s.Label
	}
	names := nameColumn(labels)
	room := 1 - names - 0.02 - 0.17
	centre := names + 0.02 + room/2
	slot := 1.0 / float64(maxInt(len(stages), 1))
	c.Height = rowsHeight(len(stages), 8.5)
	var above [2]float64
	for i, s := range stages {
		w := maxOf(s.Share, 0.02) * room
		y, h := float64(i)*slot+slot*0.2, slot*0.6
		if i > 0 {
			c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark, Tone: step(i-1) + "-wash",
				Points: [][2]float64{{above[0], y - slot*0.4}, {above[1], y - slot*0.4}, {centre + w/2, y}, {centre - w/2, y}}})
		}
		above = [2]float64{centre - w/2, centre + w/2}
		c.Marks = append(c.Marks,
			document.Mark{Kind: document.RectMark, X: centre - w/2, Y: y, W: w, H: h, Tone: step(i)},
			name(s.Label, y+h/2),
			text(1, y+h/2, fmt.Sprintf("%s · %s", s.Formatted, percent(s.Share)), "ink", document.EndAnchor, true))
	}
}

// heatmap draws the grid, which is already dense: its rows named down the
// left, its columns under it, and each cell's value where there is room.
func heatmap(c *document.Chart, b Block) {
	cols, rows := len(b.HeatColumns), len(b.HeatRows)
	if cols == 0 || rows == 0 {
		return
	}
	left := nameColumn(b.HeatRows)
	w, h := (1-left)/float64(cols), 1/float64(rows)
	c.Height = rowsHeight(rows, 7)
	for i, r := range b.HeatRows {
		c.Marks = append(c.Marks, name(r, float64(i)*h+h/2))
	}
	for _, cell := range b.Cells {
		x, y := indexOf(b.HeatColumns, cell.Column), indexOf(b.HeatRows, cell.Row)
		if x < 0 || y < 0 || cell.Empty {
			continue
		}
		shade := fmt.Sprintf("ramp-%d", minInt(cell.Step, 5)+1)
		c.Marks = append(c.Marks, document.Mark{Kind: document.RectMark,
			X: left + float64(x)*w, Y: float64(y) * h, W: w * 0.95, H: h * 0.9, Tone: shade})
		if fits(cell.Formatted, w*0.95) {
			c.Marks = append(c.Marks, text(left+float64(x)*w+w*0.475, float64(y)*h+h*0.45, cell.Formatted, inkOn(shade), "", true))
		}
	}
	c.XTicks = shifted(categories(b.HeatColumns), left)
}

// plot draws the dots of a scatter or a bubble, against both of its scales:
// a point a millimetre across, and the largest bubble about a seventh of the
// box's height, as on screen. They were three times that, and two bubbles
// filled a chart.
func plot(c *document.Chart, b Block) {
	if b.XAxis == nil || b.YAxis == nil {
		return
	}
	for _, p := range b.Points {
		r := pointRadius * 1.2
		if p.Weight > 0 {
			r = 0.006 + sqrt(p.Weight)*0.02
		}
		c.Marks = append(c.Marks, document.Mark{
			Kind: document.DotMark,
			X:    norm(p.X, b.XAxis.Min, b.XAxis.Max),
			Y:    1 - norm(p.Y, b.YAxis.Min, b.YAxis.Max),
			W:    r, Tone: tone(p.Slot), Label: p.Label, Value: p.FY,
		})
	}
	c.Ticks, c.XTicks = ticksOf(b.YAxis), ticksOf(b.XAxis)
}

// pie draws each slice as a polygon, keys each with its value and share, and
// writes a donut's whole in its middle.
//
// A polygon and not an arc: Typst has no arc primitive, and a slice at this
// size is indistinguishable from one drawn with enough vertices. Computing
// them here keeps the trigonometry out of the template.
func pie(c *document.Chart, b Block) {
	total := 0.0
	for _, s := range b.Series {
		if s.Value > 0 {
			total += s.Value
		}
	}
	if total <= 0 {
		return
	}
	donut := b.Chart == "donut"
	inner := 0.0
	if donut {
		inner = 0.58
	}
	from := -0.25 // Twelve o'clock, in turns.
	for i, s := range b.Series {
		if s.Value <= 0 {
			continue
		}
		span := s.Value / total
		c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark, Points: slice(from, span, inner),
			Tone: tone(i), Stroke: 0.8})
		from += span
	}
	c.Keys = sliceKeys(b.Series, total)
	if donut && len(b.Totals) > 0 {
		// Across most of the hole, which is inner of the box's width.
		c.Marks = append(c.Marks, headline(0.48, b.Totals[0].Formatted, 15, inner*0.78),
			text(0.5, 0.59, b.Totals[0].Label, "muted", "", false))
	}
}

// gauge draws the track and the arc over it, as the same polygons a pie uses,
// open at the bottom: the value and its share of the target in the middle, and
// the scale's two ends under the arc's.
func gauge(c *document.Chart, g *Gauge) {
	const sweep, inner = 240.0 / 360, 0.72
	// Centred on twelve o'clock, which is -0.25 of a turn here as it is for
	// a pie. It was -0.5 — nine o'clock — and every gauge printed as a "C"
	// lying on its back.
	from := -0.25 - sweep/2
	c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark, Points: slice(from, sweep, inner), Tone: "line"})
	if g.Share > 0 {
		arc := "series-1"
		if g.Over != "" {
			arc = "good"
		}
		c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark, Points: slice(from, sweep*minOf(g.Share, 1), inner), Tone: arc})
	}
	c.Marks = append(c.Marks, headline(0.48, g.Formatted, 18, inner*0.78))
	if g.Target > 0 {
		c.Marks = append(c.Marks, text(0.5, 0.61, percent(g.Value/g.Target)+" of "+strings.ToLower(g.TargetLabel), "muted", "", false))
	}
	c.Marks = append(c.Marks, text(0.13, 0.86, "0", "muted", "", false), text(0.87, 0.86, g.TargetFormatted, "muted", "", false))
	// Under the dial, as on screen: what it is measured against, and by how
	// much it beat it.
	if g.TargetFormatted != "" {
		c.Note = strings.TrimSpace(g.TargetLabel + " " + g.TargetFormatted)
	}
	if g.Over != "" {
		c.Note = strings.TrimPrefix(c.Note+" · "+g.Over, " · ")
	}
}
