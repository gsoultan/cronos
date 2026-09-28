package run

import (
	"fmt"
	"testing"
)

// A side of more than a sankey can name keeps its largest and folds the rest
// into one, carrying what they carried; a side that fits keeps every node.
func TestASankeyFoldsItsSmallestNodes(t *testing.T) {
	var nodes []side
	for i := range 15 {
		nodes = append(nodes, side{label: fmt.Sprint("n", i), value: float64(100 - i)})
	}
	at, out := rankFold(nodes)
	if len(out) != sankeyMost || out[sankeyMost-1].label != "Other" {
		t.Fatalf("%d nodes ending %+v, want %d with the rest as Other", len(out), out[len(out)-1], sankeyMost)
	}
	if want := float64(89 + 88 + 87 + 86); out[sankeyMost-1].value != want || at[14] != sankeyMost-1 {
		t.Errorf("Other carries %v, want %v", out[sankeyMost-1].value, want)
	}
	if _, fits := rankFold(nodes[:sankeyMost]); len(fits) != sankeyMost || fits[sankeyMost-1].label == "Other" {
		t.Errorf("twelve nodes were folded: %+v", fits)
	}
}
