package run

// MapKey is one entry of a map's categorical legend: a colour slot and what
// it stands for.
type MapKey struct {
	Label string `json:"label"`
	Slot  int    `json:"slot"`
}
