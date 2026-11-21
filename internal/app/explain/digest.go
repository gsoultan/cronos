package explain

import (
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/app/run"
)

/*
The bounds on what one explanation sends.

A view is already bounded — a table pages, a map gathers its cells in the
database — but bounded for a screen is not bounded for a prompt. These keep an
explanation to a few thousand tokens whatever the report holds: a long series
keeps its first and last points, which is where "it rose" or "it fell" is
read, and says how many it left out rather than pretending it had none.
*/
const (
	mostBlocks = 40
	mostPoints = 50
	mostRows   = 20
	mostText   = 120
	mostDigest = 32 << 10
)

// digest writes the view as plain text, one block after another.
//
// Text rather than the view's JSON: a model reads "Revenue: $1.2M (up 4% on
// last month)" more reliably than it reads the forty fields a chart carries
// for drawing, most of which are coordinates.
func digest(v run.View) string {
	var b strings.Builder
	line(&b, "Report: %s", v.Title)
	if v.Description != "" {
		line(&b, "Description: %s", v.Description)
	}
	for _, f := range v.Filters {
		if len(f.Values) > 0 {
			line(&b, "Filter %s: %s", f.Label, clip(strings.Join(f.Values, ", ")))
		}
	}
	for i, blk := range v.Blocks {
		if i == mostBlocks {
			line(&b, "(%d more blocks not shown)", len(v.Blocks)-i)
			break
		}
		block(&b, i, blk)
	}
	out := b.String()
	if len(out) > mostDigest {
		out = out[:mostDigest] + "\n(truncated)"
	}
	return out
}

// block writes one, headed by its place so a question about "the second
// chart" and a request focused on block 1 mean the same thing.
func block(b *strings.Builder, i int, blk run.Block) {
	kind := blk.Kind
	if blk.Chart != "" {
		kind = blk.Chart + " chart"
	}
	line(b, "\n[Block %d] %s — %s", i, clip(blk.Title), kind)
	switch blk.Kind {
	case "stat":
		stat(b, blk)
	case "table":
		table(b, blk)
	case "map":
		line(b, "A map. Its places are not listed here.")
	default:
		chart(b, blk)
	}
	if blk.Coverage != nil && len(blk.Coverage.Ignored) > 0 {
		line(b, "Filters not applied to this block: %s", strings.Join(blk.Coverage.Ignored, ", "))
	}
}

func stat(b *strings.Builder, blk run.Block) {
	line(b, "Value: %s", blk.Value)
	if d := blk.Delta; d != nil {
		verdict := "worse"
		if d.Good {
			verdict = "better"
		}
		line(b, "Change: %s %s %s (%s)", d.Dir, d.Value, d.Label, verdict)
	}
	bars(b, "Trend", blk.Trend)
}

func table(b *strings.Builder, blk run.Block) {
	heads := make([]string, len(blk.Columns))
	for i, c := range blk.Columns {
		heads[i] = clip(c.Label)
	}
	line(b, "Columns: %s", strings.Join(heads, " | "))
	for i, row := range blk.Rows {
		if i == mostRows {
			break
		}
		cells := make([]string, len(row))
		for j, c := range row {
			cells[j] = clip(c)
		}
		line(b, "%s", strings.Join(cells, " | "))
	}
	total := max(blk.Total, len(blk.Rows))
	if total > min(len(blk.Rows), mostRows) {
		line(b, "(%d rows in all; %d shown)", total, min(len(blk.Rows), mostRows))
	}
}

// line writes one line, and is the only place a newline is added — so a label
// with a newline in it cannot forge a block heading, and a value holding
// "</report>" cannot close the data early: angle brackets are written as
// look-alikes the model reads the same and the delimiters do not match.
func line(b *strings.Builder, format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	s = strings.TrimLeft(s, "\n")
	if strings.HasPrefix(format, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(unbracket.Replace(s))
	b.WriteByte('\n')
}

var unbracket = strings.NewReplacer("\n", " ", "\r", " ", "<", "‹", ">", "›")

// clip keeps a label to a sentence. An authored title is short; a cell can be
// a paragraph of free text nobody needs whole to read a trend.
func clip(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= mostText {
		return string(r)
	}
	return string(r[:mostText]) + "…"
}
