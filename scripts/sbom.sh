#!/usr/bin/env bash
#
# Writes an SPDX document for one release archive.
#
#   scripts/sbom.sh <archive.tar.gz> <out.spdx.json>
#
# Called by goreleaser, which runs it from dist/.
#
# The archive is unpacked first because syft will not read a .tar.gz of loose
# binaries: handed one directly it resolves the name as a container image, tries
# nine sources, and exits 0 having written an empty file — which is worse than
# failing, because a zero-byte SBOM published beside a release looks like an
# answer.
#
# Catalogued from the unpacked binaries rather than from go.mod. A Go binary
# carries the module list it was actually built with, so what is described is
# what shipped; go.mod describes what a build would resolve today, which is a
# different question and the wrong one to hand a security review.
set -euo pipefail

archive="${1:?usage: sbom.sh <archive> <document>}"
document="${2:?usage: sbom.sh <archive> <document>}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

tar -xzf "$archive" -C "$work"
syft scan "dir:$work" -o "spdx-json=$document" -q

# Refused rather than published. An empty document is the failure mode this
# script exists to prevent, so it is the one thing worth asserting.
if ! grep -q '"packages"' "$document" 2>/dev/null; then
	echo "sbom: $archive catalogued nothing" >&2
	exit 1
fi
