package run

import (
	"context"
	"fmt"
	"math"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// defaultBins is about how many bins a histogram cuts when its report does
// not say.
const defaultBins = 12

// histogram draws a histogram: its range surveyed, cut into bins on round
// numbers, and each bin's rows counted — two statements over the rows the
// caller may read, neither of which brings a row into Go.
func (s *Service) histogram(ctx context.Context, blk definition.Block, ds definition.Dataset,
	engine Engine, params map[string]any, filters query.Filters, pr principal.Principal) (Block, error) {

	plan, cov, err := engine.Builder.BuildBlock(ds, blk, params, filters, pr)
	if err != nil {
		return Block{}, err
	}
	lo, hi, n, err := surveyRange(ctx, engine, plan, blk)
	if err != nil {
		return Block{}, err
	}
	out := Block{Kind: string(blk.Kind), Title: blk.Heading(), Chart: string(blk.Chart),
		Series: []Bar{}, Bins: []Bin{}, Coverage: coverage(cov)}
	if n == 0 {
		return out, nil
	}
	bins := binsFor(lo, hi, orDefault(blk.Bins, defaultBins))
	p, err := engine.Builder.BuildHistogram(ds, blk, params, filters, pr, bins)
	if err != nil {
		return Block{}, err
	}
	rows, err := engine.Executor.Execute(ctx, p)
	if err != nil {
		return Block{}, fmt.Errorf("%w: block %q: %v", ErrExecute, blk.Heading(), err)
	}
	defer rows.Close()
	err = out.readBins(rows, bins)
	return out, err
}

// surveyRange reads a histogram's survey: its number's least and greatest
// value, and how many rows have one.
func surveyRange(ctx context.Context, engine Engine, plan query.Plan, blk definition.Block) (lo, hi float64, n int, err error) {
	rows, err := engine.Executor.Execute(ctx, plan)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("%w: block %q: %v", ErrExecute, blk.Heading(), err)
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, 0, 0, rows.Err()
	}
	var l, h, c any
	if err := rows.Scan(&l, &h, &c); err != nil {
		return 0, 0, 0, err
	}
	lo, _ = number(l)
	hi, _ = number(h)
	count, _ := number(c)
	return lo, hi, int(count), rows.Err()
}

// binsFor cuts a range into about want bins on round numbers — the width a
// scale's own ticks would take — so a bin reads 500–1,000 rather than
// 487.3–974.6. A range of whole numbers is cut no finer than one.
func binsFor(lo, hi float64, want int) query.Bins {
	if hi <= lo {
		// Every row one value: one bin that holds it.
		step := niceStep(math.Max(math.Abs(lo), 1) / 10)
		return query.Bins{Origin: math.Floor(lo/step) * step, Step: step, Count: 1}
	}
	step := niceStep((hi - lo) / float64(want))
	if step < 1 && lo == math.Trunc(lo) && hi == math.Trunc(hi) {
		step = 1
	}
	for {
		origin := math.Floor(lo/step) * step
		count := max(int(math.Ceil((hi-origin)/step)), 1)
		if count <= definition.MaxBins {
			return query.Bins{Origin: origin, Step: step, Count: count}
		}
		step *= 2
	}
}

// readBins reads each bin's index and value into every bin the range was cut
// into — an empty bin is a zero, not a gap — and the scales along and up.
func (out *Block) readBins(rows Rows, bins query.Bins) error {
	values := make([]float64, bins.Count)
	for rows.Next() {
		var at, v any
		if err := rows.Scan(&at, &v); err != nil {
			return err
		}
		i, _ := number(at)
		n, _ := number(v)
		if k := int(i); k >= 0 && k < bins.Count {
			values[k] = n
		}
	}
	for k, v := range values {
		from := bins.Origin + float64(k)*bins.Step
		to := from + bins.Step
		out.Bins = append(out.Bins, Bin{From: from, To: to, Label: compact(from) + "–" + compact(to),
			Value: v, Formatted: compact(v)})
	}
	x, y := edgeAxis(bins), axis(append(values, 0))
	out.XAxis, out.YAxis = &x, &y
	return rows.Err()
}

// edgeAxis is a histogram's scale along its bins: a tick on an edge, every
// second or third where there are more than a scale can say — and always on
// the last, so the range reads to its end, in place of the tick before it
// where the two would crowd. Every second edge of nine bins stopped at the
// eighth, and a range to 1,200 was labelled to 1,100.
func edgeAxis(bins query.Bins) Axis {
	end := bins.Origin + float64(bins.Count)*bins.Step
	every := max(1, int(math.Ceil(float64(bins.Count+1)/8)))
	var at []int
	for k := 0; k < bins.Count; k += every {
		at = append(at, k)
	}
	if last := at[len(at)-1]; last > 0 && bins.Count-last <= every/2 {
		at = at[:len(at)-1]
	}
	out := Axis{Min: bins.Origin, Max: end}
	for _, k := range append(at, bins.Count) {
		v := bins.Origin + float64(k)*bins.Step
		out.Ticks = append(out.Ticks, Tick{At: float64(k) / float64(bins.Count), Label: compact(v)})
	}
	return out
}

// orDefault is v, or fallback where v is not set.
func orDefault(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}
