/*
Package embedscript carries the embed element's JavaScript inside the binary.

A host application loads `<cronos-report>` from the cronos server it talks to,
so the element and the API it calls are always the same build. From npm or a
CDN they drift: a server upgraded on Tuesday meets a widget pinned in March,
and the difference shows up as an empty panel in somebody else's product.

The bundle is built by scripts/embed-bundle.sh, from packages/embed, into
bundle/ beside this file. It is not committed — a generated 37 KB file in the
history goes stale the first time someone forgets to rebuild it — so a plain
`go build` without it is a binary that serves no script, and says so at
startup. Every release channel builds it first: the Dockerfile, goreleaser's
before hook, and `make build`. scripts/check-release-parity.sh holds both
channels to that.
*/
package embedscript

import (
	"embed"
	"io/fs"
)

// bundle holds README.md always, and cronos-embed.js when it was built.
//
//go:embed bundle
var bundle embed.FS

// Name is the file a host page loads.
const Name = "cronos-embed.js"

// Bytes returns the built element, or nil when this binary was built without
// it.
func Bytes() []byte {
	b, err := fs.ReadFile(bundle, "bundle/"+Name)
	if err != nil {
		return nil
	}
	return b
}
