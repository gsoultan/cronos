package run

import "github.com/gsoultan/cronos/internal/core/definition"

// defaultBands are where a bullet's track changes shade when the report does
// not say: below 60% of the target, to 90%, and the rest.
var defaultBands = []float64{0.6, 0.9}

// readBullets reads a bullet chart's rows — a category when x is set, a
// value, and the target when it is a column — and the scale they share.
//
// One scale for every row, from nothing to the furthest value or target, so
// a longer bar is a larger number wherever it is on the chart.
func readBullets(blk definition.Block, rows Rows) ([]Bullet, *Axis, error) {
	out := []Bullet{}
	reach := []float64{0}
	for rows.Next() {
		var label, value, target any
		var dest []any
		if blk.X.Field != "" {
			dest = append(dest, &label)
		}
		dest = append(dest, &value)
		if !blk.Target.Fixed() {
			dest = append(dest, &target)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, nil, err
		}
		r := Bullet{}
		if blk.X.Field != "" {
			r.Label = bucketLabel(label, blk.X.Grain)
		}
		r.Value, _ = number(value)
		if blk.Target.Fixed() {
			r.Target = *blk.Target.Value
		} else {
			r.Target, _ = number(target)
		}
		r.Formatted, r.TargetFormatted = compact(r.Value), compact(r.Target)
		out = append(out, r)
		reach = append(reach, r.Value, r.Target)
	}
	x := axis(reach)
	return out, &x, rows.Err()
}

// bandsOf is where the track changes shade: the report's, or the defaults.
func bandsOf(blk definition.Block) []float64 {
	if len(blk.Bands) > 0 {
		return blk.Bands
	}
	return defaultBands
}
