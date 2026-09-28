package run

// Day is one day of a calendar: its date as ISO 8601, what it measured,
// formatted by the engine, and which shade of the ramp that falls in.
type Day struct {
	Date      string  `json:"date"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	Step      int     `json:"step"`
}
