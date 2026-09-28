package run

// Frames is a mark in each period of a timed map it had rows in, keyed by the
// period's place in GeoMap.Frames.
//
// A map and not a list: a place seen in one period of sixty would send
// fifty-nine nulls, and five thousand such places are most of a megabyte of
// nothing. Keyed, a map sends no more of these than it read rows.
type Frames map[int]FrameMark

// FrameMark is a mark's value in one period of a map that plays through time
// — its shade, for a shaded mark, and its size, for a sized one.
//
// Keys of one letter: a timed map sends one of these per mark per period,
// and the payload is the thing a reader waits for.
type FrameMark struct {
	Value     float64 `json:"v"`
	Formatted string  `json:"f"`
	Step      int     `json:"s,omitempty"`
	Weight    float64 `json:"w,omitempty"`
	// Size is a place's size measure in the period, on a map with one.
	Size string `json:"z,omitempty"`
}
