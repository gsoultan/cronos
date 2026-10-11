package run

import (
	"context"
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// pivotSheet writes a tab that is a crosstab: the one a pivot block draws,
// with its cells and totals as numbers a spreadsheet can add up again.
func (s *Statements) pivotSheet(ctx context.Context, r definition.Report, ds definition.Dataset,
	spec definition.Sheet, params map[string]any, pr principal.Principal) (SheetData, error) {

	blk := definition.Block{Kind: definition.PivotBlock, Value: spec.Value, Pivot: spec.Pivot}
	engine, err := s.svc.engines.Engine(ctx, ds)
	if err != nil {
		return SheetData{}, err
	}
	plan, _, err := engine.Builder.BuildBlock(ds, blk, params, query.Filters{Defs: r.Filters}, pr)
	if err != nil {
		return SheetData{}, err
	}
	rows, err := engine.Executor.Execute(ctx, plan)
	if err != nil {
		return SheetData{}, fmt.Errorf("%w: sheet %q: %v", ErrExecute, spec.Name, err)
	}
	defer rows.Close()

	x, err := readPivot(blk, ds, rows)
	if err != nil {
		return SheetData{}, err
	}
	cols, _ := x.table(blk, ds)
	return SheetData{
		Name: spec.Name, Headers: headers(cols).labels(), Rows: x.values(),
		FreezeHeader: spec.FreezeHeader, AutoFilter: spec.AutoFilter,
	}, nil
}

// values is the crosstab as cells, in the layout table gives it.
func (x crosstab) values() [][]Value {
	out := make([][]Value, 0, len(x.rows)+1)
	for r, label := range x.rows {
		row := []Value{{Text: label}}
		for c := range x.columns {
			row = append(row, sheetCell(x.cells[r][c], x.nulls[[2]int{r, c}]))
		}
		out = append(out, append(row, sheetCell(x.rowTotals[r], false)))
	}
	if len(x.rows) == 0 {
		return out
	}
	last := []Value{{Text: x.totalLabel()}}
	for _, f := range x.columnTotals {
		last = append(last, sheetCell(f, false))
	}
	return append(out, append(last, sheetCell(x.total, false)))
}

func sheetCell(f fold, null bool) Value {
	if !f.set {
		return Value{Text: pivotCell(f, null)}
	}
	return Value{Number: f.value, Numeric: true, Text: pivotCell(f, null)}
}
