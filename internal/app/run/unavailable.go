package run

// Unavailable is why a basemap cannot be drawn, worded for whoever is reading
// the report.
//
// A type rather than a sentinel, because the reason is the payload: it
// reaches the reader of an embedded report — our customer's customer — so it
// is written for them, and it names no key, secret or setting. The operator's
// version of the same fact goes to the log, where the adapter that knew it
// puts it.
type Unavailable struct {
	Reason string
}

func (u Unavailable) Error() string { return u.Reason }
