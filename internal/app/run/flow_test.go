package run_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/document"
)

/*
Sankeys and sunbursts over the depots: England sends 1,200 parcels by Aurora
and 300 by Baltic, and Scotland 700 by Aurora.
*/

// A sankey's bands meet their nodes edge to edge: each as thick at both ends,
// each source's bands stacked down it in order, and each target's arriving in
// its sources' order, so none cross inside a node.
func TestASankeyIsLaidOutEdgeToEdge(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: sankey
  title: Region to carrier
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)
	s := b.Sankey
	if s == nil || len(s.Nodes) != 4 || len(s.Links) != 3 {
		t.Fatalf("sankey %+v, want four nodes and three bands", s)
	}
	names := []string{}
	for _, n := range s.Nodes {
		names = append(names, n.Label)
	}
	if strings.Join(names, ",") != "England,Scotland,Aurora,Baltic" {
		t.Errorf("nodes %v, want each side largest first", names)
	}
	england, scotland, aurora := s.Nodes[0], s.Nodes[1], s.Nodes[2]
	first, second, third := s.Links[0], s.Links[1], s.Links[2]
	if first.Y0 != england.Y || first.Y1 != aurora.Y || math.Abs(second.Y0-(england.Y+first.H)) > 1e-12 {
		t.Errorf("England's bands do not leave it in order: %+v %+v from %+v", first, second, england)
	}
	if third.Y0 != scotland.Y || math.Abs(third.Y1-(aurora.Y+first.H)) > 1e-12 {
		t.Errorf("Scotland's band does not meet Aurora under England's: %+v", third)
	}
	if math.Abs(england.H-(first.H+second.H)) > 1e-12 || math.Abs(aurora.H-(first.H+third.H)) > 1e-12 {
		t.Errorf("a node is not as tall as its bands: %+v %+v", england, aurora)
	}
	if out := marshal(t, b); out["sankey"] == nil {
		t.Errorf("the payload has no sankey: %v", keys(out))
	}
}

// A sunburst reads its parts as a grouped chart does and prints as rings: a
// segment per series and one per category within it, in a square.
func TestASunburstPrintsAsRings(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: sunburst
  title: Carrier within region
  x: {field: carrier}
  series: {field: region}
  y: {field: parcels, aggregate: sum}`)
	if len(b.Groups) != 2 || len(b.Totals) != 1 || b.Totals[0].Formatted != "2,200" {
		t.Fatalf("%d groups and whole %+v, want a region each and 2,200", len(b.Groups), b.Totals)
	}
	c := run.Printable(run.View{Blocks: []run.Block{b}})[0]
	if !c.Square || len(marksOf(c, document.PolyMark)) != 5 || len(c.Keys) != 2 {
		t.Errorf("square %v, %d segments, keys %+v", c.Square, len(marksOf(c, document.PolyMark)), c.Keys)
	}
}

// A sankey prints its bands and names both sides of it.
func TestASankeyPrints(t *testing.T) {
	c := run.Printable(run.View{Blocks: []run.Block{draw(t, `- kind: chart
  chart: sankey
  title: Region to carrier
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)}})[0]
	if len(marksOf(c, document.PolyMark)) != 3 || len(marksOf(c, document.RectMark)) != 4 ||
		!strings.Contains(words(c), "England · 1,500") || !strings.Contains(words(c), "Aurora · 1,900") {
		t.Errorf("%d bands, %d nodes, words %q", len(marksOf(c, document.PolyMark)),
			len(marksOf(c, document.RectMark)), words(c))
	}
}

func TestAFlowNeedsItsSecondDimension(t *testing.T) {
	for _, chart := range []string{"sankey", "sunburst"} {
		err := load(`- kind: chart
  chart: ` + chart + `
  title: t
  x: {field: region}
  y: {field: parcels}`)
		if !errors.Is(err, definition.ErrInvalid) || !strings.Contains(err.Error(), "needs series") {
			t.Errorf("a %s without series: %v", chart, err)
		}
	}
}
