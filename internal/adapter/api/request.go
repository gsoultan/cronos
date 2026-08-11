package api

import "github.com/gsoultan/cronos/internal/core/query"

// request is what an embedded viewer sends.
//
// Filters and params, and nothing else. Notably not a principal, an
// organization or a project: those come from the token, and accepting them
// here would mean a client could ask to be someone.
type request struct {
	Filters map[string]query.FilterValue `json:"filters,omitempty"`
	Params  map[string]any               `json:"params,omitempty"`
	// Output picks a profile. Empty means the first interactive one, which is
	// what a viewer wants without knowing what the author called it.
	Output string `json:"output,omitempty"`
}
