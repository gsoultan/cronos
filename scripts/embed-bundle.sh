#!/usr/bin/env bash
#
# Builds the embed element and puts it where cronosd's go:embed reads it.
#
# Run before any `go build` whose binary should serve
# /v1/embed/cronos-embed.js: `make build`, goreleaser's before hook and
# scripts/live-embed.sh call it, and the Dockerfile does the same two steps in a
# stage of its own. A binary built without it still runs, serves no element, and
# logs `embed-script=false` at startup.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT=internal/platform/embedscript/bundle/cronos-embed.js

# The workspace install, because packages/embed builds the chart renderers from
# packages/charts by path. Frozen: a release that resolves a version for itself
# ships something nobody chose.
[ -d node_modules ] || bun install --frozen-lockfile
(cd packages/embed && bun run build)

cp packages/embed/dist/cronos-embed.js "$OUT"
# An empty file would embed, mount the route and serve a script that does
# nothing — the failure that says least about itself.
test -s "$OUT" || { echo "embed-bundle: $OUT is empty" >&2; exit 1; }
echo "embed-bundle: $(wc -c < "$OUT" | tr -d ' ') bytes -> $OUT"
