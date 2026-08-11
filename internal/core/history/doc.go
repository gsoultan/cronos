// Package history is the record of what was actually delivered.
//
// docs/product.md calls this the enterprise wedge: an auditor asks what a
// particular customer received last March and expects an answer, not a
// reconstruction. That needs three things recorded together — which definition
// version ran, what period it covered, and where each document went — because
// any two of them without the third is a story rather than evidence.
//
// A run is written when it starts, not when it finishes. A burst that crashed
// halfway is exactly the run somebody needs to look at, and one that only
// exists once it succeeded is a log of successes.
package history
