package query

import (
	"fmt"
	"math"
	"strconv"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
A part of the world, as a filter.

A map sets it — the view a reader has zoomed to, or a distance around a place —
and every block reading a dataset the filter binds narrows to the same part of
the world: the table under the map, the totals over it, the other maps.

Every number arrives from a browser and is bound as an argument. The fields are
the author's, checked against the dataset like every other filter's.
*/

// earthKm is the Earth's mean diameter, in the haversine below.
const earthKm = 12742.0

// area compiles an area filter's value against ds.
func (b *binder) area(ds definition.Dataset, def definition.Filter, v FilterValue) (string, error) {
	lat, lon, ok := def.Coordinates(ds.Name)
	if !ok || (v.Op != Within && v.Op != Near) {
		return "", fmt.Errorf("%w: filter %q is not an area, or was sent %q, which an area "+
			"does not take — within or near", ErrBadArgument, def.Name, v.Op)
	}
	for _, f := range []string{lat, lon} {
		if _, ok := ds.Field(f); !ok {
			return "", fmt.Errorf("%w: filter %q binds %s to %q, which is not a field of it",
				ErrBadTemplate, def.Name, ds.Name, f)
		}
	}
	if err := checkArity(def, v); err != nil {
		return "", err
	}
	n, err := numbers(def, v.Values)
	if err != nil {
		return "", err
	}
	if v.Op == Within {
		return b.within(def, lat, lon, n)
	}
	return b.near(def, lat, lon, n)
}

// within is a box, south, west, north and east. A west east of its east
// crosses the antimeridian, and takes both sides of it.
func (b *binder) within(def definition.Filter, lat, lon string, n []float64) (string, error) {
	south, west, north, east := n[0], n[1], n[2], n[3]
	if !onGlobe(south, west) || !onGlobe(north, east) || south > north {
		return "", fmt.Errorf("%w: filter %q was sent a box that is not on the globe",
			ErrBadArgument, def.Name)
	}
	lats := fmt.Sprintf("%s BETWEEN %s AND %s", lat, b.value(south), b.value(north))
	if west <= east {
		return fmt.Sprintf("(%s AND %s BETWEEN %s AND %s)", lats, lon, b.value(west), b.value(east)), nil
	}
	return fmt.Sprintf("(%s AND (%s >= %s OR %s <= %s))", lats, lon, b.value(west), lon, b.value(east)), nil
}

/*
near is everything within a distance of a place, by the haversine — compared on
its inner term rather than taken through ASIN and SQRT, because the comparison
is the same and the functions are not: an arcsine a rounding error past one is
an error in Postgres. A box around the circle goes first, so an index on the
coordinates is used and the trigonometry runs only on rows that could be in it.
*/
func (b *binder) near(def definition.Filter, lat, lon string, n []float64) (string, error) {
	at, on, km := n[0], n[1], n[2]
	if !onGlobe(at, on) || !(km > 0) || km > earthKm*math.Pi/2 {
		return "", fmt.Errorf("%w: filter %q was sent a place or a distance that is not on the globe",
			ErrBadArgument, def.Name)
	}
	reach := math.Pow(math.Sin(km/earthKm), 2)
	// The box only chooses which rows the exact comparison runs on, so it is
	// drawn a hundredth larger than the circle: too large costs a few rows,
	// too small loses places at the circle's edge.
	d := 2 * km / earthKm
	dLat := d * 180 / math.Pi * 1.01
	box := fmt.Sprintf("%s BETWEEN %s AND %s", lat, b.value(math.Max(-90, at-dLat)),
		b.value(math.Min(90, at+dLat)))
	// The longitudes a circle reaches widen towards the poles. One reaching
	// over a pole reaches every longitude, and one across the antimeridian
	// wraps: for those the circle alone decides.
	if s := math.Sin(d) / math.Cos(at*math.Pi/180); s < 1 {
		if dLon := math.Asin(s) * 180 / math.Pi * 1.01; on-dLon >= -180 && on+dLon <= 180 {
			box += fmt.Sprintf(" AND %s BETWEEN %s AND %s", lon, b.value(on-dLon), b.value(on+dLon))
		}
	}
	// Each term bound where it is written, twice for a square: a positional
	// placeholder is consumed where it appears, so one value cannot stand in
	// two places on SQLite or MySQL the way $1 can on Postgres.
	sin := func(col string, v float64) string {
		return fmt.Sprintf("SIN((%s - %s) * 8.726646259971648E-3)", col, b.value(v))
	}
	const rad = "1.7453292519943295E-2"
	return fmt.Sprintf("(%s AND %s * %s + COS(%s * %s) * COS(%s * %s) * %s * %s <= %s)",
		box, sin(lat, at), sin(lat, at), b.value(at), rad, lat, rad,
		sin(lon, on), sin(lon, on), b.value(reach)), nil
}

// numbers reads a filter's values as finite numbers — from JSON, where they
// arrive as float64, or from a query string, where they arrive as text.
func numbers(def definition.Filter, vs []any) ([]float64, error) {
	out := make([]float64, len(vs))
	for i, v := range vs {
		var f float64
		switch t := v.(type) {
		case float64:
			f = t
		case int:
			f = float64(t)
		case string:
			var err error
			if f, err = strconv.ParseFloat(t, 64); err != nil {
				return nil, fmt.Errorf("%w: filter %q was sent %q, which is not a number",
					ErrBadArgument, def.Name, t)
			}
		default:
			return nil, fmt.Errorf("%w: filter %q was sent %v, which is not a number",
				ErrBadArgument, def.Name, v)
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("%w: filter %q was sent a number that is not finite",
				ErrBadArgument, def.Name)
		}
		out[i] = f
	}
	return out, nil
}

func onGlobe(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}
