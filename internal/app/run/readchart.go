package run

// readGroups reads (bucket, series, value) into one Group per series.
//
// The result is dense: every group carries every bucket, in the order the
// buckets first appeared, with a zero where that series had no row. A stacked
// chart cannot add a column up otherwise, and a grouped one cannot keep its
// bars in the same order under each label — and doing it here means each
// renderer does not have to.
func readGroups(rows Rows, grain string) ([]Group, error) {
	// key is one cell of the dense matrix: which series, which bucket.
	type key struct{ series, bucket string }

	var order, series []string
	seenBucket, seenSeries := map[string]bool{}, map[string]bool{}
	values := map[key]float64{}

	for rows.Next() {
		var rawBucket, rawSeries, rawValue any
		if err := rows.Scan(&rawBucket, &rawSeries, &rawValue); err != nil {
			return nil, err
		}
		bucket, name := bucketLabel(rawBucket, grain), cell(rawSeries)
		if !seenBucket[bucket] {
			seenBucket[bucket] = true
			order = append(order, bucket)
		}
		if !seenSeries[name] {
			seenSeries[name] = true
			series = append(series, name)
		}
		n, _ := number(rawValue)
		// Summed rather than assigned. Two rows can land in one cell when a
		// date grain buckets them together, and the last one winning would
		// silently drop the other.
		values[key{name, bucket}] += n
	}

	slot := slots(series, CategorySlots)
	// Empty rather than nil, for the reason readPoints is.
	out := make([]Group, 0, len(series))
	for _, name := range series {
		bars := make([]Bar, 0, len(order))
		for _, b := range order {
			v := values[key{name, b}]
			bars = append(bars, Bar{Label: b, Value: v, Formatted: compact(v)})
		}
		out = append(out, Group{Label: name, Slot: slot[name], Bars: bars})
	}
	return out, nil
}

// readPoints reads (label, x, y[, series][, size]) into the dots of a plot.
//
// The columns present depend on the block, so the count is read from the
// result rather than assumed — the same contract the map reader uses, and for
// the same reason: the compiler and the reader have to agree about a shape
// that varies, and only one of them can be the source of it.
func readPoints(rows Rows, hasSeries, hasSize bool) ([]Point, Axis, Axis, error) {
	cols := 3
	if hasSeries {
		cols++
	}
	if hasSize {
		cols++
	}

	// Empty rather than nil, so the key is emitted even when nothing matched —
	// the same absent-versus-empty rule the rest of the payload follows.
	out := []Point{}
	var names []string
	var sizes, xs, ys []float64
	for rows.Next() {
		cells := make([]any, cols)
		into := make([]any, cols)
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := rows.Scan(into...); err != nil {
			return nil, Axis{}, Axis{}, err
		}

		x, _ := number(cells[1])
		y, _ := number(cells[2])
		p := Point{Label: cell(cells[0]), X: x, Y: y, FX: compact(x), FY: compact(y)}

		at := 3
		if hasSeries {
			names = append(names, cell(cells[at]))
			at++
		} else {
			names = append(names, p.Label)
		}
		if hasSize {
			s, _ := number(cells[at])
			p.Size, sizes = compact(s), append(sizes, s)
		}
		xs, ys = append(xs, x), append(ys, y)
		out = append(out, p)
	}

	paint(out, names, sizes, hasSize)
	return out, axis(xs), axis(ys), nil
}

// paint assigns each dot its colour slot and, for a bubble, its radius.
//
// Separate from the scan because both need every row: the largest bubble is
// not known until the last one has arrived, and a slot assigned per row would
// depend on which rows the database happened to return first.
func paint(points []Point, names []string, sizes []float64, hasSize bool) {
	slot := slots(names, PlotSlots)
	lo, hi := span(sizes)
	for i := range points {
		points[i].Slot = slot[names[i]]
		if hasSize {
			points[i].Weight = weight(sizes[i], lo, hi)
		}
	}
}

// totals is the height of each stack, formatted here for the reason every
// other displayable value is — see Block.Totals.
func totals(groups []Group) []Bar {
	if len(groups) == 0 {
		return []Bar{}
	}
	out := make([]Bar, 0, len(groups[0].Bars))
	for i, bucket := range groups[0].Bars {
		sum := 0.0
		for _, g := range groups {
			sum += g.Bars[i].Value
		}
		out = append(out, Bar{Label: bucket.Label, Value: sum, Formatted: compact(sum)})
	}
	return out
}
