package run

import (
	"github.com/gsoultan/cronos/internal/core/definition"
)

// Dumbbell is a dumbbell chart's row: a category, the value it ran from and
// the value it ran to, read along XAxis.
type Dumbbell struct {
	Label string  `json:"label"`
	From  float64 `json:"from"`
	To    float64 `json:"to"`
	// FromSaid and ToSaid are the values as text, and Change the difference
	// signed, by the engine that knew the unit.
	FromSaid string `json:"fromSaid"`
	ToSaid   string `json:"toSaid"`
	Change   string `json:"change"`
}

// readDumbbells reads (bucket, m0, m1) into a row each, with the one scale
// both ends are read along.
func readDumbbells(blk definition.Block, rows Rows) ([]Dumbbell, []string, *Axis, error) {
	tracks, _, _, err := readCombo(blk, rows)
	if err != nil || len(tracks) != 2 {
		return nil, nil, nil, err
	}
	out := make([]Dumbbell, len(tracks[0].Bars))
	reach := []float64{}
	for i, from := range tracks[0].Bars {
		to := tracks[1].Bars[i]
		out[i] = Dumbbell{Label: from.Label, From: from.Value, To: to.Value,
			FromSaid: from.Formatted, ToSaid: to.Formatted, Change: signed(to.Value-from.Value, compact)}
		reach = append(reach, from.Value, to.Value)
	}
	x := axis(reach)
	return out, []string{tracks[0].Label, tracks[1].Label}, &x, nil
}

// signed is a change as text with its sign, so a rise and a fall read apart.
func signed(v float64, said func(float64) string) string {
	switch {
	case v > 0:
		return "+" + said(v)
	case v < 0:
		return "−" + said(-v)
	}
	return said(0)
}

// writeDumbbells formats each end by its own metric, and the change and the
// scale by the first's.
func (b *Block) writeDumbbells(blk definition.Block, ds definition.Dataset) {
	if len(blk.Metrics) != 2 {
		return
	}
	from, to := formatOf(ds, blk.Metrics[0].Field), formatOf(ds, blk.Metrics[1].Field)
	for i := range b.Dumbbells {
		d := &b.Dumbbells[i]
		d.FromSaid, d.ToSaid = writeOne(from, d.From, d.FromSaid), writeOne(to, d.To, d.ToSaid)
		if !to.plain() {
			d.Change = signed(d.To-d.From, to.short)
		}
	}
	writeAxis(b.XAxis, from)
	// Each end by what the dataset calls it, where the report gave no word.
	for i, m := range blk.Metrics {
		if m.Label == "" && i < len(b.Ends) {
			b.Ends[i] = labelOf(ds, m.Field)
		}
	}
}
