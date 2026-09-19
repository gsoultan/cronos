package run

// Shape is one polygon of a choropleth.
//
// Path is an SVG `d` in Web Mercator world units — see geoPath for why the
// projection happens here and not in the browser.
type Shape struct {
	Label     string  `json:"label"`
	Path      string  `json:"path"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	// Step is which stop of the sequential ramp shades this shape, from 0 for
	// the lightest. A step and not a colour: the viewer's ramp is a set of CSS
	// custom properties, which is the embed's whole theming API, and a hex
	// chosen on the server would ignore the host's dark mode.
	Step int `json:"step"`
}
