#!/usr/bin/env bash
#
# The two-factor flow, through a real browser, against a real server.
#
# Everything else that checks this drives the API. This drives what a person
# touches, and it earned its place the first time it ran: the enrolment wizard
# was rendering inside the account page, between the password and the sessions,
# with two sets of Back/Continue on screen — and two sibling panels were keyed by
# the same counter, so React was quietly discarding one. Neither is visible to a
# type checker, a linter, or an API test.
#
#   ./scripts/live-portal-2fa.sh
#
# Needs go, bun, and playwright's chromium (bunx playwright install chromium).
# Starts its own server and its own portal on a fresh database, so the account
# it enrols has never had a second factor before.
set -euo pipefail

cd "$(dirname "$0")/.."

API_PORT=8798
PORTAL_PORT=5273
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

# lsof exits 1 when it finds nothing, which under `set -e` and `pipefail` ends
# the script — silently, because the failure is a pipeline rather than a
# command. That cost half an hour: the script stopped after printing "portal"
# and said nothing about why.
freeport() {
	local pids
	pids=$(lsof -ti ":$1" 2>/dev/null || true)
	[ -n "$pids" ] && { kill $pids 2>/dev/null || true; }
	return 0
}

inuse() { [ -n "$(lsof -ti ":$1" 2>/dev/null || true)" ]; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

say "cronos"
go build -o bin/cronosd ./cmd/cronosd || die "build"
mkdir -p "$work/defs"

# A port already in use is somebody else's server, and enrolling against it
# would be enrolling into their database.
inuse "$API_PORT" && die "something is already on $API_PORT"

CRONOS_ADDR=":$API_PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_ORIGINS="$PORTAL" \
	./bin/cronosd >"$work/server.log" 2>&1 &
server=$!

for _ in $(seq 1 40); do
	curl -sf "$API/v1/health" >/dev/null 2>&1 && break
	sleep 0.25
done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/server.log"; die "cronos never came up"; }

printf 'a-password-they-chose' | go run ./cmd/cronos-user \
	-dsn "file:$work/c.db" -driver sqlite -email ada@acme.example \
	-name "Ada Rahayu" -role admin -org default -project default >/dev/null 2>&1 ||
	die "could not create the account"
printf '  on %s, with one account and no second factor\n' "$API"

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

say "Enrolling, in a browser"
(cd apps/portal && PORTAL="$PORTAL" bun run e2e) || die "the browser check"
