#!/usr/bin/env bash
# Enforces the license boundary against the real build graph.
#
#   1. The BSL binary (cmd/cronosd) must not depend on ee/, at any depth.
#   2. Core packages must not import ee/. Dependencies point one way only.
#
# Run in CI on every push. A violation here means BSL-licensed artifacts would
# ship commercially-licensed code.
set -euo pipefail

MODULE=$(go list -m)
EE_PREFIX="${MODULE}/ee"
status=0

echo "==> ${MODULE}"

# 1. Transitive dependency check on the community binary.
if leaked=$(go list -deps ./cmd/cronosd | grep "^${EE_PREFIX}\(/\|$\)" || true); [ -n "$leaked" ]; then
	echo "FAIL: cmd/cronosd depends on commercially-licensed packages:" >&2
	printf '  %s\n' $leaked >&2
	status=1
else
	echo "ok: cmd/cronosd does not reach ee/"
fi

# 2. Direct import check. Only ee/ itself and the EE binary may import ee/.
offenders=$(go list -f '{{$p := .ImportPath}}{{range .Imports}}{{$p}} -> {{.}}
{{end}}{{range .TestImports}}{{$p}} -> {{.}}
{{end}}' ./... |
	awk -v ee="$EE_PREFIX" -v eecmd="${MODULE}/cmd/cronosd-ee" '
		function under(pkg, root) { return pkg == root || index(pkg, root "/") == 1 }
		NF == 3 && under($3, ee) && !under($1, ee) && !under($1, eecmd) { print $1 " -> " $3 }
	')

if [ -n "$offenders" ]; then
	echo "FAIL: core packages import ee/:" >&2
	printf '  %s\n' "$offenders" >&2
	status=1
else
	echo "ok: no core package imports ee/"
fi

# 3. ee/ must carry its own license.
if [ ! -f ee/LICENSE ]; then
	echo "FAIL: ee/LICENSE is missing" >&2
	status=1
else
	echo "ok: ee/LICENSE present"
fi

exit $status
