package run

// Credit is one part of a basemap's credit line.
type Credit struct {
	Text string `json:"text"`
	// Href is where the part links to, or empty for plain text. Only ever a
	// provider's own page, written by cronos: an author's URL basemap has no
	// credits, only an attribution, so nothing here comes from a definition.
	Href string `json:"href,omitempty"`
}
