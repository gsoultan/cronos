package definition

// DimensionRef points a block at something to group by.
type DimensionRef struct {
	Field string `json:"field" yaml:"field"`
	// Grain buckets a date: day, week, month, quarter, year.
	Grain string `json:"grain,omitempty" yaml:"grain,omitempty"`
}
