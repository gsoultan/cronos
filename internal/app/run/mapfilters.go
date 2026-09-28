package run

import (
	"context"
	"strconv"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
setsFilters works out which of the report's filters a map can set, and what
they hold now.

A pick is a string or enum filter bound, in the map's dataset, to the field the
map labels by — so the label a reader clicks is a value the filter takes as it
is. An area is an area filter bound to the map's own latitude and longitude,
so the box a reader zooms to is a box in the same two fields.
*/
func setsFilters(m *GeoMap, r definition.Report, blk definition.Block, ds definition.Dataset,
	values map[string]query.FilterValue) {

	if f, ok := pickFilter(r, blk, ds); ok {
		m.Pick = &MapPick{Filter: f.Name, Values: texts(values[f.Name])}
	}
	if f, ok := areaFilter(r, blk, ds); ok {
		m.Area = areaOf(f.Name, values[f.Name])
	}
}

// pickFilter is the filter a click on this map sets, if there is one.
func pickFilter(r definition.Report, blk definition.Block, ds definition.Dataset) (definition.Filter, bool) {
	// A label a reader can click is the value only when it is text: a number
	// arrives on the map formatted, and "12,345" is not a value of anything.
	if label, _ := ds.Field(blk.Labels()); label.Type != "string" {
		return definition.Filter{}, false
	}
	for _, f := range r.Filters {
		if f.Type != definition.String && f.Type != definition.Enum {
			continue
		}
		if field, ok := f.Binds(ds.Name); ok && field == blk.Labels() {
			return f, true
		}
	}
	return definition.Filter{}, false
}

// areaFilter is the area filter over this map's own coordinates, if there is one.
func areaFilter(r definition.Report, blk definition.Block, ds definition.Dataset) (definition.Filter, bool) {
	if blk.Map == nil {
		return definition.Filter{}, false
	}
	for _, f := range r.Filters {
		if lat, lon, ok := f.Coordinates(ds.Name); ok && lat == blk.Map.Lat && lon == blk.Map.Lon {
			return f, true
		}
	}
	return definition.Filter{}, false
}

/*
control is the filters a map is drawn under in a browser: the report's, less
the one a click on it sets.

A map that picks is that filter's control. It shows every place the filter can
be set to, with the picked one marked, the way a dropdown lists every value
and not only the chosen one — narrowed by its own pick it would show a single
place, and no way to pick another but to let go of the first. The rest of the
report, and whatever is drawn over the map, is narrowed as always; so is the
map on paper, where nothing can be clicked.

A pick is a reader's choice, never a constraint on them: a host pins
parameters rather than filters, and the reader's scope is applied under every
query whatever filters it carries. Leaving one out narrows less, and cannot
reach a row the reader could not already see.
*/
func (s *Service) control(ctx context.Context, r definition.Report, out definition.Output,
	blk definition.Block, f query.Filters) query.Filters {

	if blk.Map == nil || out.Renderer != definition.Interactive {
		return f
	}
	ds, err := s.datasets.Dataset(ctx, blk.DatasetFor(r.Dataset))
	if err != nil {
		return f
	}
	p, ok := pickFilter(r, blk, ds)
	if _, set := f.Values[p.Name]; !ok || !set {
		return f
	}
	values := make(map[string]query.FilterValue, len(f.Values))
	for name, v := range f.Values {
		if name != p.Name {
			values[name] = v
		}
	}
	return query.Filters{Defs: f.Defs, Values: values}
}

// texts is a pick filter's values, when it is picking — `in` or `eq`.
func texts(v query.FilterValue) []string {
	if v.Op != query.In && v.Op != query.Eq {
		return nil
	}
	out := make([]string, 0, len(v.Values))
	for _, x := range v.Values {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// areaOf is an area filter and what it holds, when it holds something a map
// can draw.
func areaOf(name string, v query.FilterValue) *MapArea {
	a := &MapArea{Filter: name}
	if v.Op != query.Within && v.Op != query.Near {
		return a
	}
	for _, x := range v.Values {
		switch t := x.(type) {
		case float64:
			a.Values = append(a.Values, t)
		case string:
			f, err := strconv.ParseFloat(t, 64)
			if err != nil {
				return &MapArea{Filter: name}
			}
			a.Values = append(a.Values, f)
		default:
			return &MapArea{Filter: name}
		}
	}
	a.Op = string(v.Op)
	return a
}
