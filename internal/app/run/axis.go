package run

// Axis is the scale a viewer draws beside a plot.
//
// Ticks are computed and formatted here rather than in the browser, which is
// the same decision as every other displayable value on the wire: the engine
// knows the currency and the locale, and a tick reading "2400" beside a tile
// reading "$2.4k" is the disagreement that makes a reader distrust both.
type Axis struct {
	// Min and Max are the raw ends of the scale, so a viewer can place a point
	// without re-deriving the bounds the ticks were chosen for.
	Min float64 `json:"min"`
	Max float64 `json:"max"`
	// Ticks run from Min to Max, each carrying where it sits as a fraction.
	Ticks []Tick `json:"ticks"`
}
