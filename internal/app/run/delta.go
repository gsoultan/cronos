package run

// Delta is how a stat moved: its last period against the one before, as a
// share formatted by the engine, which way it went, and whether that way is
// good news — which only the definition knows, because outstanding invoices
// rising is bad and revenue rising is good.
type Delta struct {
	Value string `json:"value"`
	Dir   string `json:"dir"`
	Good  bool   `json:"good"`
	Label string `json:"label,omitempty"`
}
