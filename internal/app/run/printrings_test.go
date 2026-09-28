package run

import (
	"math"
	"testing"

	"github.com/gsoultan/cronos/internal/core/document"
)

// A radius printed around a place is the distance on the ground in every
// direction: each vertex, taken back off the page to a latitude and a
// longitude, is the radius from the place — far north as at the equator.
func TestAPrintedRadiusIsTheDistanceOnTheGround(t *testing.T) {
	for _, place := range [][2]float64{{4.9, 52.37}, {18.96, 69.65}, {-43.2, -22.9}} {
		x, y := project(place[0], place[1])
		m := &GeoMap{Bounds: Bounds{MinX: x - 0.01, MinY: y - 0.01, MaxX: x + 0.01, MaxY: y + 0.01},
			Layers: []string{"radius"}, Markers: []Marker{{Label: "Depot", X: x, Y: y}}, RadiusKm: 10}
		c := &document.Chart{Title: "Catchments"}
		printMap(c, m)
		var wash *document.Mark
		for i := range c.Marks {
			if c.Marks[i].Tone == "series-2-wash" {
				wash = &c.Marks[i]
			}
		}
		if wash == nil || len(wash.Points) != ringSides {
			t.Fatalf("around %v: no circle printed: %+v", place, c.Marks)
		}
		for _, p := range wash.Points {
			wx, wy := m.Bounds.MinX+p[0]*0.02, m.Bounds.MinY+p[1]*0.02
			d := greatCircleKm(place[1], place[0], latitude(wy), wx*360-180)
			if math.Abs(d-10) > 1e-6 {
				t.Errorf("around %v a vertex is %.9f km away, want 10", place, d)
			}
		}
	}
}

// A printed flow says which way it goes, as the screen does: a head where it
// lands, pointing along the way it arrives.
func TestAPrintedFlowEndsInAHeadPointingItsWay(t *testing.T) {
	c := &document.Chart{Title: "Transfers"}
	at := func(x, y float64) [2]float64 { return [2]float64{x, y} }
	flows(c, []Arc{{Label: "A → B", X1: 0.1, Y1: 0.5, X2: 0.9, Y2: 0.5}}, at, false)
	if len(c.Marks) != 2 || c.Marks[1].Kind != document.PolyMark || len(c.Marks[1].Points) != 3 {
		t.Fatalf("marks %+v, want the line and its head", c.Marks)
	}
	head := c.Marks[1].Points
	if head[0] != [2]float64{0.9, 0.5} {
		t.Errorf("the head's point is at %v, want the destination", head[0])
	}
	// Its base is behind its point, the way the flow came from.
	if base := (head[1][0] + head[2][0]) / 2; base >= 0.9 {
		t.Errorf("the head's base is at x %v, not behind its point", base)
	}
}

func greatCircleKm(lat1, lon1, lat2, lon2 float64) float64 {
	r := math.Pi / 180
	h := math.Pow(math.Sin((lat2-lat1)*r/2), 2) +
		math.Cos(lat1*r)*math.Cos(lat2*r)*math.Pow(math.Sin((lon2-lon1)*r/2), 2)
	return 2 * 6371 * math.Asin(math.Sqrt(h))
}
