package run_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
A map that plays through time: every period a frame, in time order whatever
order the rows came in; every mark's value in each, and none where it had no
rows; and the map as it opens every period together, as the rest of the
report shows them.
*/

// timedService is a table of deliveries on days, and a map over it.
func timedService(t *testing.T, rows []string, block string) (*run.Service, definition.Report) {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:timed%d?mode=memory&cache=shared", charts.Add(1)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	stmt := "CREATE TABLE drops (id TEXT, region TEXT, shape TEXT, lat REAL, lon REAL, parcels REAL, day TEXT);\n" +
		"INSERT INTO drops VALUES " + strings.Join(rows, ",\n") + ";"
	if _, err := db.Exec(stmt); err != nil {
		t.Fatal(err)
	}
	ds, err := yamlcodec.Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: drops}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, region, shape, lat, lon, parcels, day FROM drops
  fields:
    - {name: id,      type: string,  role: dimension}
    - {name: region,  type: string,  role: dimension}
    - {name: shape,   type: string,  role: dimension}
    - {name: lat,     type: decimal, role: dimension}
    - {name: lon,     type: decimal, role: dimension}
    - {name: parcels, type: decimal, role: measure, aggregate: sum}
    - {name: day,     type: date,    role: dimension}`))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: timed}
spec:
  dataset: drops
  outputs:
    - name: interactive
      renderer: interactive
      layout:
` + indent(block)))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	return run.New(datasets{"drops": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
	}}), rep
}

const square = `'{"type":"Polygon","coordinates":[[[4.8,52.3],[5.0,52.3],[5.0,52.4],[4.8,52.4],[4.8,52.3]]]}'`

func TestAPlaceIsOneMarkWithAValueInEachPeriod(t *testing.T) {
	// Out of order, and B has nothing on the second day.
	s, rep := timedService(t, []string{
		"('d1', 'A', '', 52.37, 4.90, 5, '2026-08-03')",
		"('d2', 'A', '', 52.37, 4.90, 2, '2026-08-01')",
		"('d3', 'B', '', 52.38, 4.91, 4, '2026-08-01')",
		"('d4', 'A', '', 52.37, 4.90, 1, '2026-08-02')",
		"('d5', 'B', '', 52.38, 4.91, 8, '2026-08-03')",
	}, `- kind: chart
  chart: map
  title: Drops by day
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map: {layers: [bubble], lat: lat, lon: lon, time: {field: day, grain: day}}`)
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	m := v.Blocks[0].Map
	if len(m.Frames) != 3 || !strings.Contains(m.Frames[0], "1") || !strings.Contains(m.Frames[2], "3") {
		t.Fatalf("periods %v, want the three days in order", m.Frames)
	}
	if len(m.Markers) != 2 {
		t.Fatalf("%d markers, want one for each place", len(m.Markers))
	}
	for _, mk := range m.Markers {
		switch mk.Label {
		case "A":
			if mk.Value != 8 || frameValues(mk.Frames, 3) != "2 1 5" {
				t.Errorf("A: total %v, periods %s", mk.Value, frameValues(mk.Frames, 3))
			}
		case "B":
			if mk.Value != 12 || frameValues(mk.Frames, 3) != "4 - 8" {
				t.Errorf("B: total %v, periods %s", mk.Value, frameValues(mk.Frames, 3))
			}
			// Sized against the largest in any period: 8 is the largest.
			if mk.Frames[2].Weight != 1 {
				t.Errorf("B's 8 weighs %v, want the full size", mk.Frames[2].Weight)
			}
		}
	}
}

// A row with no date is in the map as it opens, and in no period.
func TestARowWithNoDateIsInNoPeriod(t *testing.T) {
	s, rep := timedService(t, []string{
		"('d1', 'A', '', 52.37, 4.90, 5, '2026-08-01')",
		"('d2', 'A', '', 52.37, 4.90, 2, NULL)",
	}, `- kind: chart
  chart: map
  title: Drops by day
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map: {layers: [scatter], lat: lat, lon: lon, time: {field: day, grain: day}}`)
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	m := v.Blocks[0].Map
	if len(m.Frames) != 1 || m.Markers[0].Value != 7 || frameValues(m.Markers[0].Frames, 1) != "5" {
		t.Errorf("periods %q, A %v in all and %s in them — want one day, 7 and 5",
			m.Frames, m.Markers[0].Value, frameValues(m.Markers[0].Frames, 1))
	}
}

// A place's size adds up across its periods as its value does, by its own
// aggregate, and each period says its own.
func TestAPlaceIsSizedAcrossItsPeriods(t *testing.T) {
	s, rep := timedService(t, []string{
		"('d1', 'A', '', 52.37, 4.90, 5, '2026-08-03')",
		"('d2', 'A', '', 52.37, 4.90, 2, '2026-08-01')",
		"('d3', 'A', '', 52.37, 4.90, 1, '2026-08-01')",
	}, `- kind: chart
  chart: map
  title: Drops by day
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  size: {field: parcels, aggregate: count}
  map: {layers: [bubble], lat: lat, lon: lon, time: {field: day, grain: day}}`)
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	a := v.Blocks[0].Map.Markers[0]
	if a.Value != 8 || a.Size != "3" {
		t.Errorf("A is %v sized %q, want 8 sized 3 — every drop, not its first day's", a.Value, a.Size)
	}
	if len(a.Frames) != 2 || a.Frames[0].Size != "2" || a.Frames[1].Size != "1" {
		t.Errorf("A's days are sized %+v %+v, want 2 then 1", a.Frames[0], a.Frames[1])
	}
}

// One set of shades for every period, so a colour means one value in all of
// them — and a legend for the periods apart from the one for the whole.
func TestARegionIsShadedAlikeInEveryPeriod(t *testing.T) {
	var rows []string
	for d := 1; d <= 6; d++ {
		rows = append(rows, fmt.Sprintf("('d%d', 'Zone', %s, 52.35, 4.9, %d, '2026-%02d-15')", d, square, d*10, d))
	}
	s, rep := timedService(t, rows, `- kind: chart
  chart: map
  title: Parcels by month
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map: {layers: [polygon], geometry: shape, time: {field: day, grain: month}}`)
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	m := v.Blocks[0].Map
	zone := m.Shapes[0]
	if zone.Value != 210 || len(zone.Frames) != 6 || len(m.FrameLegend) == 0 {
		t.Fatalf("zone total %v, %d periods, frame legend %v", zone.Value, len(zone.Frames), m.FrameLegend)
	}
	if zone.Frames[0].Step >= zone.Frames[5].Step {
		t.Errorf("January's 10 is shade %d and June's 60 shade %d", zone.Frames[0].Step, zone.Frames[5].Step)
	}
}

// Every period, however many: a map that played the latest few would open on
// totals over periods its reader could not play. A place sends only the
// periods it had rows in.
func TestAMapPlaysEveryPeriod(t *testing.T) {
	var rows []string
	for d := 0; d < 90; d++ {
		rows = append(rows, fmt.Sprintf("('d%d', 'A', '', 52.37, 4.90, 1, date('2026-01-01', '+%d days'))", d, d))
	}
	rows = append(rows, "('b', 'B', '', 52.38, 4.91, 7, '2026-02-01')")
	s, rep := timedService(t, rows, `- kind: chart
  chart: map
  title: Drops by day
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map: {layers: [scatter], lat: lat, lon: lon, time: {field: day, grain: day}}`)
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	m := v.Blocks[0].Map
	if len(m.Frames) != 90 || !strings.Contains(m.Frames[0], "Jan") || !strings.Contains(m.Frames[89], "Mar") {
		t.Fatalf("%d periods from %q to %q, want the 90 days of the first quarter",
			len(m.Frames), m.Frames[0], m.Frames[len(m.Frames)-1])
	}
	for _, mk := range m.Markers {
		want := map[string]int{"A": 90, "B": 1}[mk.Label]
		if len(mk.Frames) != want {
			t.Errorf("%s sends %d periods, want the %d it had rows in", mk.Label, len(mk.Frames), want)
		}
	}
	if b := markerOf(m, "B"); b.Frames[31].Value != 7 {
		t.Errorf("B's 1 February is %+v, want 7", b.Frames[31])
	}
}

func markerOf(m *run.GeoMap, label string) run.Marker {
	for _, mk := range m.Markers {
		if mk.Label == label {
			return mk
		}
	}
	return run.Marker{}
}

// frameValues is a mark's value in each of n periods, "-" where it had none.
func frameValues(frames run.Frames, n int) string {
	var out []string
	for i := range n {
		f, ok := frames[i]
		if !ok {
			out = append(out, "-")
			continue
		}
		out = append(out, fmt.Sprint(f.Value))
	}
	return strings.Join(out, " ")
}
