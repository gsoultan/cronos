#!/usr/bin/env bash
#
# First-run setup: verifies the toolchain and installs dependencies.
# Safe to re-run.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORTAL="$ROOT/apps/portal"

if [ -t 1 ]; then
	OK=$'\033[38;5;35m✓\033[0m'; BAD=$'\033[38;5;203m✗\033[0m'; DIM=$'\033[2m'; OFF=$'\033[0m'
else
	OK='ok'; BAD='!!'; DIM=''; OFF=''
fi

missing=0

# Compares dotted versions without sort -V, which BSD sort lacks.
version_at_least() {
	awk -v have="$1" -v want="$2" 'BEGIN {
		n = split(have, h, "."); split(want, w, ".")
		for (i = 1; i <= 3; i++) {
			hi = (i <= n ? h[i] + 0 : 0); wi = w[i] + 0
			if (hi > wi) exit 0
			if (hi < wi) exit 1
		}
		exit 0
	}'
}

require() {
	local name="$1" min="$2" got="$3" url="$4"
	if [ -z "$got" ]; then
		printf '%s %-8s not found %s— %s%s\n' "$BAD" "$name" "$DIM" "$url" "$OFF"
		missing=1
	elif version_at_least "$got" "$min"; then
		printf '%s %-8s %s\n' "$OK" "$name" "$got"
	else
		printf '%s %-8s %s %s(need >= %s — %s)%s\n' "$BAD" "$name" "$got" "$DIM" "$min" "$url" "$OFF"
		missing=1
	fi
}

echo
echo "Toolchain"
require go 1.26 \
	"$(go version 2>/dev/null | awk '{print $3}' | sed 's/^go//')" \
	"https://go.dev/dl/"
require bun 1.3 \
	"$(bun --version 2>/dev/null)" \
	"https://bun.sh"
# The paginated renderer shells out to typst. Required rather than optional:
# without it the PDF tests do not fail, they skip, and a renderer nobody
# exercised is a renderer nobody trusts.
require typst 0.15 \
	"$(typst --version 2>/dev/null | awk '{print $2}')" \
	"brew install typst — or https://github.com/typst/typst/releases"

[ "$missing" = 1 ] && { echo; echo "Install the missing tools above, then re-run."; exit 1; }

echo
echo "Dependencies"
(cd "$ROOT" && go mod download) && printf '%s go modules\n' "$OK"
(cd "$PORTAL" && bun install >/dev/null 2>&1) && printf '%s portal packages\n' "$OK"
(cd "$ROOT" && bun install >/dev/null 2>&1) && printf '%s embed + react packages\n' "$OK"

echo
echo "Verifying"
(cd "$ROOT" && go build ./... ) && printf '%s go build\n' "$OK"
(cd "$ROOT" && go test ./internal/... >/dev/null) && printf '%s go tests\n' "$OK"
(cd "$ROOT" && ./scripts/check-license-boundary.sh >/dev/null) && printf '%s license boundary\n' "$OK"
(cd "$PORTAL" && bunx tsc --noEmit) && printf '%s portal typecheck\n' "$OK"
(cd "$ROOT/packages/embed" && bunx tsc --noEmit) && printf '%s embed typecheck\n' "$OK"
(cd "$ROOT/packages/react" && bunx tsc --noEmit) && printf '%s react typecheck\n' "$OK"

echo
echo "Ready. Run:  make dev"
echo
