package run

/*
MapPick is the report filter a click on a map sets: the one bound to the field
the map labels its regions and places by. Clicking a region narrows every block
the filter reaches to it — the table under the map, the totals over it — and
clicking it again lets go.

Values are what the filter holds now, so a viewer can tell a click that picks
from one that lets go, and mark what is picked.
*/
type MapPick struct {
	Filter string   `json:"filter"`
	Values []string `json:"values,omitempty"`
}
