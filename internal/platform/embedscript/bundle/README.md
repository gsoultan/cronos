# bundle

`cronos-embed.js` lands here, built from `packages/embed` by
`scripts/embed-bundle.sh`, and is compiled into `cronosd` with `go:embed`.

It is ignored by git on purpose. This file is what keeps the directory — and
so the `go:embed` pattern — valid in a checkout that has not built it.
