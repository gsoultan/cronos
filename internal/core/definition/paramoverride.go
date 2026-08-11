package definition

// ParamOverride narrows a dataset parameter for one report.
//
// Pin is the difference between a default and a constraint. An unpinned
// override is a starting value a caller may replace; a pinned one is the
// report's own decision and the API rejects an attempt to change it. Without
// the distinction every default is silently overridable, which is fine until
// one of them was load-bearing.
type ParamOverride struct {
	Default any  `json:"default,omitempty" yaml:"default,omitempty"`
	Pin     bool `json:"pin,omitempty" yaml:"pin,omitempty"`
}
