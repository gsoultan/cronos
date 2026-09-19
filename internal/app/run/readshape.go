package run

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// readWaterfall reads (bucket, value) into floating bars and a closing total.
//
// The running total is accumulated here rather than in the viewer, so the last
// bar lands where the arithmetic says and not where a chain of float additions
// done twice in two languages happens to put it.
func readWaterfall(blk definition.Block, rows Rows) ([]Step, Axis, error) {
	out := []Step{}
	running := 0.0
	lo, hi := 0.0, 0.0

	for rows.Next() {
		var label, value any
		if err := rows.Scan(&label, &value); err != nil {
			return nil, Axis{}, err
		}
		v, _ := number(value)
		start := running
		running += v
		out = append(out, Step{
			Label: bucketLabel(label, blk.X.Grain), Value: v, Formatted: compact(v),
			Start: start, End: running, Sign: signOf(v),
		})
		lo, hi = min(lo, running), max(hi, running)
	}
	if err := rows.Err(); err != nil {
		return nil, Axis{}, err
	}

	if len(out) > 0 {
		// The closing bar, drawn from the baseline. A waterfall without one
		// asks the reader to add the steps up, which is the arithmetic the
		// chart exists to have done for them.
		out = append(out, Step{
			Label: "Total", Value: running, Formatted: compact(running),
			Start: 0, End: running, Sign: 0, Total: true,
		})
	}
	return out, axis([]float64{lo, hi}), nil
}

func signOf(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// HeatAxis caps how many rows and columns a heatmap draws.
//
// The grid is dense, so its size is the *product* of two cardinalities while
// ChartLimit only bounds their sum: five thousand rows each carrying a distinct
// pair is a twenty-five-million-cell grid, from a query that returned five
// thousand rows. `GROUP BY customer_id, invoice_id` does exactly that, and the
// block that did it looks reasonable to whoever wrote it.
//
// Sixty is also about where a heatmap stops being readable, so this is a cap
// on a chart nobody could have read rather than on one somebody wanted.
const HeatAxis = 60

// grid is a heatmap's cells and the axes they are placed on.
type grid struct {
	cells   []Cell
	rows    []string
	columns []string
	// full is how many cells the ungapped grid would have held, before the
	// axes were capped. Saying so beats letting somebody conclude the rows
	// that were cut had nothing in them.
	//
	// Not the count of pairs that matched: the grid is dense, so that number
	// is smaller than the cells drawn and comparing the two says nothing.
	full int
}

// readHeatmap reads (column, row, value) into a dense grid.
//
// Dense, and every missing pair marked empty rather than dropped: a sparse
// grid makes a viewer decide what an absent cell means, and "no rows matched"
// and "matched, and the total was nothing" are different answers that should
// not look alike.
func readHeatmap(blk definition.Block, rows Rows) (grid, error) {
	type at struct{ row, column string }

	// Empty rather than nil: these are the grid's axes, and a viewer reading
	// `heatRows.map` on a report that matched nothing should get an empty
	// grid rather than a crash.
	columns, rowLabels := []string{}, []string{}
	seenCol, seenRow := map[string]bool{}, map[string]bool{}
	values := map[at]float64{}

	for rows.Next() {
		var rawColumn, rawRow, rawValue any
		if err := rows.Scan(&rawColumn, &rawRow, &rawValue); err != nil {
			return grid{}, err
		}
		column, row := bucketLabel(rawColumn, blk.X.Grain), cell(rawRow)
		if !seenCol[column] {
			seenCol[column] = true
			columns = append(columns, column)
		}
		if !seenRow[row] {
			seenRow[row] = true
			rowLabels = append(rowLabels, row)
		}
		v, _ := number(rawValue)
		values[at{row, column}] += v
	}
	if err := rows.Err(); err != nil {
		return grid{}, err
	}

	// Truncated in the order the compiler returned them, which is the order
	// the author's own sort — or the buckets themselves — put them in.
	out := grid{full: len(rowLabels) * len(columns)}
	out.rows, out.columns = clip(rowLabels), clip(columns)

	present := make([]float64, 0, len(values))
	for _, v := range values {
		present = append(present, v)
	}
	breaks := steps(present)

	out.cells = make([]Cell, 0, len(out.rows)*len(out.columns))
	for _, r := range out.rows {
		for _, c := range out.columns {
			v, ok := values[at{r, c}]
			out.cells = append(out.cells, Cell{
				Row: r, Column: c, Value: v, Formatted: compact(v),
				Step: stepOf(v, breaks), Empty: !ok,
			})
		}
	}
	return out, nil
}

func clip(labels []string) []string {
	if len(labels) > HeatAxis {
		return labels[:HeatAxis]
	}
	return labels
}

// readGauge reads one number and the target it is read against.
func readGauge(blk definition.Block, rows Rows) (*Gauge, error) {
	out := &Gauge{TargetLabel: blk.Target.Heading()}
	if !rows.Next() {
		return out, rows.Err()
	}

	var value, target any
	dest := []any{&value}
	if !blk.Target.Fixed() {
		dest = append(dest, &target)
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, err
	}

	out.Value, _ = number(value)
	if blk.Target.Fixed() {
		out.Target = *blk.Target.Value
	} else {
		out.Target, _ = number(target)
	}
	out.Formatted = compact(out.Value)
	out.TargetFormatted = compact(out.Target)

	if out.Target != 0 {
		share := out.Value / out.Target
		// Capped, because an arc cannot draw 180% — it would wrap past its own
		// start and read as 80%. The overshoot is said in words instead, which
		// is the news the reader wanted.
		out.Share = max(0, min(1, share))
		if share > 1 {
			out.Over = fmt.Sprintf("%.0f%% over", (share-1)*100)
		}
	}
	return out, rows.Err()
}
