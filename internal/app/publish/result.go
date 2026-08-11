package publish

// Result is what a successful publish returns.
//
// Version is content-addressed, so re-publishing an unchanged document returns
// the same one. A run record naming a version can then be replayed against the
// exact bytes that produced it, which is the whole of the reproducibility
// story — see docs/product.md, E7.
type Result struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version"`
}
