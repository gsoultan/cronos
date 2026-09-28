package run

import (
	"context"
	"fmt"
	"math"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// trendMost is as many periods as a stat's line draws: the latest, the rest
// being more than a tile's width can tell apart.
const trendMost = 60

// trended draws a stat with its trend: the figure as any stat's, then its
// measure per period of the trend's date — the same statement a line chart of
// it would be, over the same scoped rows — and the last period against the
// one before as the change the tile reports.
func (s *Service) trended(ctx context.Context, blk definition.Block, ds definition.Dataset,
	engine Engine, params map[string]any, filters query.Filters, pr principal.Principal) (Block, error) {

	out, err := s.run(ctx, blk, ds, engine, params, filters, pr)
	if err != nil {
		return Block{}, err
	}
	line := definition.Block{Kind: definition.ChartBlock, Chart: definition.LineChart,
		Dataset: blk.Dataset, Filter: blk.Filter, Title: blk.Heading(),
		X: definition.DimensionRef{Field: blk.Trend.Field, Grain: blk.Trend.Grain}, Y: blk.Value}
	plan, _, err := engine.Builder.BuildBlock(ds, line, params, filters, pr)
	if err != nil {
		return Block{}, err
	}
	rows, err := engine.Executor.Execute(ctx, plan)
	if err != nil {
		return Block{}, fmt.Errorf("%w: block %q: %v", ErrExecute, blk.Heading(), err)
	}
	defer rows.Close()
	points, err := readSeries(rows, blk.Trend.Grain)
	if err != nil {
		return Block{}, err
	}
	out.Trend = points[max(0, len(points)-trendMost):]
	out.Delta = deltaOf(out.Trend, blk.Trend.Lower())
	return out, nil
}

// deltaOf is the last period against the one before: nothing where there is
// no period before, or it was nothing — a change from zero has no share.
func deltaOf(points []Bar, lower bool) *Delta {
	if len(points) < 2 {
		return nil
	}
	last, before := points[len(points)-1], points[len(points)-2]
	if before.Value == 0 {
		return nil
	}
	change := (last.Value - before.Value) / math.Abs(before.Value)
	dir := "up"
	if change < 0 {
		dir = "down"
	}
	return &Delta{Value: fmt.Sprintf("%+.1f%%", change*100), Dir: dir,
		Good: (change >= 0) != lower, Label: "vs " + before.Label}
}
