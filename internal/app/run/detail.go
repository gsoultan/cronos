package run

/*
Detail is how a viewer asks for more of a large map.

The block by its place in the output, and the output by name, because a view is
a question about the report as rendered: the same parameters, the same filters,
the same reader's scope — which the viewer sends again as it sent them the first
time, and the server applies again rather than trusting anything it is sent
back.

Categories are the colours as the map opened: each view is its own query, and
the colour a category gets cannot come from the order that view's rows arrive
in, or a carrier would change colour as the reader pans. The viewer returns
these with every view and the server colours by them.
*/
type Detail struct {
	Output     string   `json:"output"`
	Block      int      `json:"block"`
	Categories []string `json:"categories,omitempty"`
}
