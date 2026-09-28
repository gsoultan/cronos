package run

import "github.com/gsoultan/cronos/internal/core/definition"

// readBoxes reads a box plot's categories, eight numbers each, and the scale
// they are read against — over where the boxes are rather than from nothing.
func readBoxes(blk definition.Block, rows Rows) ([]Box, Axis, error) {
	out := []Box{}
	var reach []float64
	for rows.Next() {
		var label any
		f := make([]any, 7) // n, low, q1, median, q3, high, outliers
		var dest []any
		if blk.X.Field != "" {
			dest = append(dest, &label)
		}
		for i := range f {
			dest = append(dest, &f[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, Axis{}, err
		}
		b := boxOf(f)
		if blk.X.Field != "" {
			b.Label = bucketLabel(label, blk.X.Grain)
		}
		out = append(out, b)
		reach = append(reach, b.Low, b.High)
	}
	return out, spanAxis(reach), rows.Err()
}

// boxOf is one category's box from its folded columns, formatted.
func boxOf(f []any) Box {
	num := func(i int) float64 {
		v, _ := number(f[i])
		return v
	}
	b := Box{N: int(num(0)), Low: num(1), Q1: num(2), Median: num(3), Q3: num(4), High: num(5),
		Outliers: int(num(6))}
	b.Said = [5]string{compact(b.Low), compact(b.Q1), compact(b.Median), compact(b.Q3), compact(b.High)}
	return b
}
