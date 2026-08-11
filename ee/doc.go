// Package ee wires commercially-licensed features into the core's extension
// seams. Importing it for side effects is the only supported entry point:
//
//	import _ "github.com/gsoultan/cronos/ee"
//
// Code in this directory is licensed under ee/LICENSE, not the repository's
// BSL. Two rules keep that boundary real:
//
//  1. ee/ may import the core. The core may never import ee/.
//  2. cmd/cronosd (the BSL binary) must not reach ee/ transitively.
//
// scripts/check-license-boundary.sh enforces both against the actual build
// graph, so a violation fails CI rather than shipping.
package ee
