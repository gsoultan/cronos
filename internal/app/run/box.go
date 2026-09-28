package run

// Box is one category's spread in a box plot: how many rows it read, the
// middle half of them as the box from Q1 to Q3, the median across it, and
// the whiskers reaching the furthest rows inside Tukey's fences — one and a
// half interquartile ranges either side of the box. Outliers counts the rows
// beyond them, which are drawn as a number rather than as dots: a dot per row
// is a payload per row.
type Box struct {
	Label    string  `json:"label"`
	N        int     `json:"n"`
	Low      float64 `json:"low"`
	Q1       float64 `json:"q1"`
	Median   float64 `json:"median"`
	Q3       float64 `json:"q3"`
	High     float64 `json:"high"`
	Outliers int     `json:"outliers,omitempty"`
	// Said is the five as the engine formats them, low to high: whisker,
	// quartile, median, quartile, whisker.
	Said [5]string `json:"said"`
}
