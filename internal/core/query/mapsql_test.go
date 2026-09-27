package query

import (
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// drops is a dataset of delivery points whose measure averages by default —
// the default a block that names no aggregate inherits without saying so.
func drops() definition.Dataset {
	return definition.Dataset{
		Name: "drops", Sources: []definition.SourceRef{{Ref: "warehouse"}},
		Query: "SELECT drop_id, lat, lon, carrier, parcels FROM drops",
		Fields: []definition.Field{
			{Name: "drop_id", Type: "string", Role: definition.Dimension},
			{Name: "lat", Type: "decimal", Role: definition.Dimension},
			{Name: "lon", Type: "decimal", Role: definition.Dimension},
			{Name: "carrier", Type: "string", Role: definition.Dimension},
			{Name: "parcels", Type: "decimal", Role: definition.Measure, Aggregate: "avg"},
		},
	}
}

func dropMap(layers ...definition.MapLayer) definition.Block {
	return definition.Block{
		Kind: definition.ChartBlock, Chart: definition.MapChart, Title: "Drops",
		X:   definition.DimensionRef{Field: "drop_id"},
		Y:   definition.MeasureRef{Field: "parcels", Aggregate: "sum"},
		Map: &definition.MapSpec{Layers: layers, Lat: "lat", Lon: "lon"},
	}
}

// A category is a dimension like any other: grouped by, so each point keeps
// the colour of the carrier it belongs to rather than being folded across two.
func TestAMapColouredByCategoryGroupsByIt(t *testing.T) {
	blk := dropMap(definition.ScatterLayer)
	blk.Series = definition.DimensionRef{Field: "carrier"}

	plan, _, err := NewBuilder(SQLite{}).BuildBlock(drops(), blk, nil, Filters{}, embedded("c-9"))
	if err != nil {
		t.Fatal(err)
	}
	sql := plan.SQL()
	if !strings.Contains(sql, "carrier AS series") {
		t.Errorf("the category is not selected:\n%s", sql)
	}
	if !strings.Contains(sql, "GROUP BY drop_id, lat, lon, carrier") {
		t.Errorf("the category is not grouped by:\n%s", sql)
	}
}

// definition refuses `avg` on a hexbin layer when the block says it. A block
// that says nothing inherits the dataset's — and that one only this package
// can see.
func TestAHexbinOverAnInheritedAverageIsRefused(t *testing.T) {
	blk := dropMap(definition.HexbinLayer)
	blk.Y.Aggregate = ""

	_, _, err := NewBuilder(SQLite{}).BuildBlock(drops(), blk, nil, Filters{}, embedded("c-9"))
	if err == nil {
		t.Fatal("hexagons averaged from averages compiled")
	}
	if !strings.Contains(err.Error(), "hexagons") || !strings.Contains(err.Error(), `"avg"`) {
		t.Errorf("the error does not say why: %v", err)
	}
}
