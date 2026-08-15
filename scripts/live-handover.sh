#!/usr/bin/env bash
#
# One browser, two people, two organisations.
#
# Everything else that checks tenancy drives the API, where each request carries
# its own token and the answer is whatever that token is entitled to. The
# browser is a second place the answers live, and it is per page load rather
# than per session: sign out, sign in as somebody in another organisation, and
# the portal has two sessions in one cache.
#
# It found what it was written to look for. No query key named who asked, so the
# second person's Reports page listed the first person's reports under the
# second person's organisation — and with a stale time on the query, no request
# went out at all, so nothing in any log says it happened.
#
#   ./scripts/live-handover.sh
#
# Needs go, bun, and playwright's chromium (bunx playwright install chromium).
# Stands up two projects in one deployment with visibly different catalogues,
# so "the wrong reports" is a thing you can see rather than infer.
set -euo pipefail

cd "$(dirname "$0")/.."

API_PORT=8799
PORTAL_PORT=5274
API="http://localhost:$API_PORT"
PORTAL="http://localhost:$PORTAL_PORT"

work=$(mktemp -d)
cleanup() {
	[ -n "${server:-}" ] && { kill "$server" 2>/dev/null || true; }
	[ -n "${portal:-}" ] && { kill "$portal" 2>/dev/null || true; }
	# The dev server forks; the port is the only reliable handle on it.
	freeport "$PORTAL_PORT"
	rm -rf "$work" || true
	return 0
}
trap 'cleanup 2>/dev/null' EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

# lsof exits 1 when it finds nothing, which under `set -e` and `pipefail` ends
# the script silently — the failure is a pipeline rather than a command.
freeport() {
	local pids
	pids=$(lsof -ti ":$1" 2>/dev/null || true)
	[ -n "$pids" ] && { kill $pids 2>/dev/null || true; }
	return 0
}
inuse() { [ -n "$(lsof -ti ":$1" 2>/dev/null || true)" ]; }

say "cronos, with two projects"
go build -o bin/cronosd ./cmd/cronosd || die "build"
inuse "$API_PORT" && die "something is already on $API_PORT"

# Two catalogues that cannot be mistaken for each other. Everything acme has is
# named after what it is; globex has one report called "Globex only", so a page
# showing the wrong one is obvious in a screenshot rather than a diff.
#
# The keys are nested under `report:`, so these replace the values rather than
# anchoring at the start of a line — the first version did anchor, matched
# nothing, and gave globex a verbatim copy of acme's report. Which the check
# then reported as a leak, correctly and for the wrong reason.
mkdir -p "$work/defs/acme/finance" "$work/defs/globex/ops"
cp demo/definitions/*.yaml "$work/defs/acme/finance/"
sed -e 's/name: billing-summary/name: globex-only/' \
	-e 's/title: Billing summary/title: Globex only/' \
	demo/definitions/billing-summary.yaml >"$work/defs/globex/ops/globex-only.yaml"
grep -q 'name: globex-only' "$work/defs/globex/ops/globex-only.yaml" ||
	die "globex's report is still named after acme's — the two catalogues would be identical"

CRONOS_ADDR=":$API_PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_PROJECTS="acme/finance,globex/ops" \
	CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_ORIGINS="$PORTAL" \
	./bin/cronosd >"$work/server.log" 2>&1 &
server=$!

for _ in $(seq 1 60); do
	curl -sf "$API/v1/health" >/dev/null 2>&1 && break
	sleep 0.25
done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/server.log"; die "cronos never came up"; }

for who in "ada@acme.example:Ada Rahayu:acme:finance:a-password-for-ada" \
	"rin@globex.example:Rin Abadi:globex:ops:a-password-for-rin"; do
	IFS=: read -r email name org project password <<<"$who"
	printf '%s' "$password" | go run ./cmd/cronos-user \
		-dsn "file:$work/c.db" -driver sqlite -email "$email" \
		-name "$name" -role admin -org "$org" -project "$project" >/dev/null 2>&1 ||
		die "could not create $email"
done
printf '  on %s — ada in acme/finance, rin in globex/ops\n' "$API"

say "portal"
freeport "$PORTAL_PORT"
sleep 1
(cd apps/portal && VITE_CRONOS_API="$API" bun run dev --port "$PORTAL_PORT" \
	>"$work/portal.log" 2>&1) &
portal=$!

for _ in $(seq 1 60); do
	curl -sf "$PORTAL/" >/dev/null 2>&1 && break
	sleep 0.5
done
curl -sf "$PORTAL/" >/dev/null || { tail -20 "$work/portal.log"; die "the portal never came up"; }
printf '  on %s\n' "$PORTAL"

say "Handing the browser over"
(cd apps/portal && PORTAL="$PORTAL" node e2e/handover.mjs) || die "the browser check"
