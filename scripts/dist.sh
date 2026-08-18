#!/usr/bin/env bash
#
# Cross-compiles the release archives.
#
#   ./scripts/dist.sh                 # every platform
#   PLATFORMS=linux/amd64 ./scripts/dist.sh
#
# A container image is one way to run cronos and not the only one: a deployment
# that already has systemd, a package mirror and a backup story wants binaries,
# not a runtime to adopt. Both channels have to carry the same set of commands,
# or a tool exists in one and not the other and the documentation is wrong for
# half its readers — which is what happened to cronos-import.
#
# Two archives per platform, because there are two licenses. cronosd is BSL and
# cronosd-ee is commercial, and the boundary that check-license-boundary.sh
# enforces in the build graph has to hold in distribution too: shipping them in
# one tarball would put commercially-licensed code in the artifact people
# download expecting the community edition. Each archive carries its own
# LICENSE, next to the binary it covers.
#
# Needs go. Writes dist/ and nothing else.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
PLATFORMS="${PLATFORMS:-linux/amd64 linux/arm64 darwin/arm64}"
OUT="dist"

# The community edition. cronos-import is here rather than only in the image:
# the person migrating four hundred .jrxml files is as likely to be on a host
# as in a container, and a migration tool that only one channel carries is a
# migration tool half the deployments cannot run.
BSL_CMDS="cronosd cronos-token cronos-user cronos-import"
EE_CMDS="cronosd-ee"

if [ -t 1 ]; then
	OK=$'\033[38;5;35m✓\033[0m'; DIM=$'\033[2m'; OFF=$'\033[0m'
else
	OK='ok'; DIM=''; OFF=''
fi
say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

# Stamped the same way the Makefile and the Dockerfile stamp it, so a binary
# from any of the three answers -version identically.
LDFLAGS="-s -w -X github.com/gsoultan/cronos/internal/platform/build.version=${VERSION}"

rm -rf "$OUT"
mkdir -p "$OUT"

# archive builds one tarball: a name, a license, and the commands in it.
archive() {
	local edition="$1" license="$2" os="$3" arch="$4"; shift 4
	local name="cronos${edition:+-$edition}_${VERSION}_${os}_${arch}"
	local stage="$OUT/$name"

	mkdir -p "$stage"
	for cmd in "$@"; do
		# CGO off everywhere: the release binary has to run on a host that is
		# not the one that built it, and a glibc version is not something a
		# download should depend on. Federation needs cgo and is a build tag;
		# it is not in a release archive for the same reason.
		CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
			go build -trimpath -ldflags="$LDFLAGS" -o "$stage/$cmd" "./cmd/$cmd" ||
			die "$cmd for $os/$arch"
	done
	cp "$license" "$stage/LICENSE"
	cp CHANGELOG.md "$stage/CHANGELOG.md"

	tar -czf "$OUT/$name.tar.gz" -C "$OUT" "$name"
	rm -rf "$stage"
	printf '  %s %s %s(%s)%s\n' "$OK" "$name.tar.gz" "$DIM" \
		"$(du -h "$OUT/$name.tar.gz" | cut -f1 | tr -d ' ')" "$OFF"
}

say "cronos $VERSION"
for platform in $PLATFORMS; do
	os="${platform%%/*}"; arch="${platform##*/}"
	archive ""   LICENSE    "$os" "$arch" $BSL_CMDS
	archive "ee" ee/LICENSE "$os" "$arch" $EE_CMDS
done

# Checksums, because a tarball downloaded over a link somebody pasted is a
# tarball nobody can verify. Computed over the archives only, so the file can
# be signed and published beside them.
say "checksums"
(cd "$OUT" && shasum -a 256 ./*.tar.gz > SHA256SUMS) 2>/dev/null ||
	(cd "$OUT" && sha256sum ./*.tar.gz > SHA256SUMS)
printf '  %s SHA256SUMS %s(%s archives)%s\n' "$OK" "$DIM" "$(ls "$OUT"/*.tar.gz | wc -l | tr -d ' ')" "$OFF"

say "dist/"
ls -1 "$OUT"
