package run

// Sankey is a flow chart already laid out: its nodes down the left and the
// right, and a band from each source to each target it sends to, in fractions
// of the box, so the screen and the page draw the same picture.
//
// Laid out here for a treemap's reason: arranging bands to meet their nodes
// edge to edge is arithmetic with one right answer, and a viewer and a
// typesetter each doing it would be two answers.
type Sankey struct {
	Nodes []SankeyNode `json:"nodes"`
	Links []SankeyLink `json:"links"`
}
