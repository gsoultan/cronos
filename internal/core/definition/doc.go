// Package definition holds the artifacts a user authors: what a dataset is,
// what may be asked of it, and who may see which rows.
//
// It has no dependencies. A definition does not know how to run, render or
// deliver itself — those are use cases that read it. Keeping this package
// inert is what lets validation be exhaustive without being slow, and lets a
// definition be checked on save rather than discovered on a Tuesday morning
// burst.
package definition
