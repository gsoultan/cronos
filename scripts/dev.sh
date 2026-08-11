#!/usr/bin/env bash
#
# Runs cronos for local development: the Go API and the portal dev server side
# by side, with prefixed output and one Ctrl-C that stops both.
#
#   scripts/dev.sh              both
#   scripts/dev.sh --web        portal only
#   scripts/dev.sh --api        API only
#
# Ports come from CRONOS_API_PORT / CRONOS_WEB_PORT.
#
# Written for bash 3.2 — the version macOS ships — so no `wait -n` and no
# associative arrays.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORTAL="$ROOT/apps/portal"
API_PORT="${CRONOS_API_PORT:-8080}"
WEB_PORT="${CRONOS_WEB_PORT:-5173}"

RUN_API=1
RUN_WEB=1
case "${1:-}" in
	--web|--web-only) RUN_API=0 ;;
	--api|--api-only) RUN_WEB=0 ;;
	'') ;;
	-h|--help) sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
	*) echo "unknown option: $1 (try --help)" >&2; exit 2 ;;
esac

if [ -t 1 ]; then
	C_API=$'\033[38;5;33m'; C_WEB=$'\033[38;5;36m'
	C_DIM=$'\033[2m'; C_ERR=$'\033[38;5;203m'; C_OFF=$'\033[0m'
else
	C_API=''; C_WEB=''; C_DIM=''; C_ERR=''; C_OFF=''
fi

note() { printf '%s│%s %s\n' "$C_DIM" "$C_OFF" "$*"; }
fail() { printf '%s✗%s %s\n' "$C_ERR" "$C_OFF" "$*" >&2; exit 1; }

# -- Preflight ---------------------------------------------------------------

command -v go >/dev/null 2>&1 || fail "go not found — https://go.dev/dl/"
if [ "$RUN_WEB" = 1 ]; then
	command -v bun >/dev/null 2>&1 || fail "bun not found — curl -fsSL https://bun.sh/install | bash"
	[ -d "$PORTAL/node_modules" ] || {
		note "installing portal dependencies…"
		(cd "$PORTAL" && bun install) || fail "bun install failed"
	}
fi

port_owner() { lsof -ti:"$1" -sTCP:LISTEN 2>/dev/null | head -1; }

check_port() {
	local port="$1" what="$2" pid
	pid="$(port_owner "$port")"
	[ -z "$pid" ] && return 0
	fail "port $port ($what) is already in use by pid $pid — \`kill $pid\`, or set ${3}=<other port>"
}

[ "$RUN_API" = 1 ] && check_port "$API_PORT" api CRONOS_API_PORT
[ "$RUN_WEB" = 1 ] && check_port "$WEB_PORT" portal CRONOS_WEB_PORT

# -- Teardown ----------------------------------------------------------------

PIDS=''
STOPPING=0

stop() {
	[ "$STOPPING" = 1 ] && return
	STOPPING=1
	printf '\n'
	note 'stopping…'
	for pid in $PIDS; do
		kill "$pid" 2>/dev/null
	done
	# Vite and `go run` both spawn children that do not die with the parent.
	for port in "$API_PORT" "$WEB_PORT"; do
		owner="$(port_owner "$port")"
		[ -n "$owner" ] && kill "$owner" 2>/dev/null
	done
	wait 2>/dev/null
	note 'stopped'
}
trap stop INT TERM EXIT

# -- Launch ------------------------------------------------------------------

prefix() {
	local tag="$1" color="$2" line
	while IFS= read -r line; do
		printf '%s%s%s %s\n' "$color" "$tag" "$C_OFF" "$line"
	done
}

start() {
	local tag="$1" color="$2" dir="$3"; shift 3
	( cd "$dir" && exec "$@" ) > >(prefix "$tag" "$color") 2>&1 &
	PIDS="$PIDS $!"
	eval "PID_${tag}=$!"
}

printf '\n'
note "cronos dev"
[ "$RUN_API" = 1 ] && note "  api    http://localhost:$API_PORT"
[ "$RUN_WEB" = 1 ] && note "  portal http://localhost:$WEB_PORT"
printf '\n'

if [ "$RUN_API" = 1 ]; then
	# The demo definitions over the demo seed, so the first report someone
	# opens has real numbers in it rather than an empty state.
	CRONOS_ADDR=":$API_PORT" \
	CRONOS_SIGNING_KEY="${CRONOS_SIGNING_KEY:-development-key-at-least-32-bytes-long}" \
	CRONOS_DEFINITIONS="${CRONOS_DEFINITIONS:-demo/definitions}" \
	CRONOS_SEED="${CRONOS_SEED:-demo/seed.sql}" \
	CRONOS_ORIGINS="${CRONOS_ORIGINS:-http://localhost:$WEB_PORT}" \
		start api "$C_API" "$ROOT" go run ./cmd/cronosd
fi
if [ "$RUN_WEB" = 1 ]; then
	# The binary directly, not `bun run vite` — the wrapper survives long enough
	# to print an exit-code complaint every time you Ctrl-C.
	start web "$C_WEB" "$PORTAL" "$PORTAL/node_modules/.bin/vite" \
		--port "$WEB_PORT" --strictPort
fi

# -- Supervise ---------------------------------------------------------------
#
# One side going down does not take the other with it: a stub or crashed API
# should not end a session you were using to work on the portal. Exits are
# reported once, loudly, and the survivor keeps running.

reported_api=0
reported_web=0

while :; do
	[ "$STOPPING" = 1 ] && break
	alive=0
	for pid in $PIDS; do
		if kill -0 "$pid" 2>/dev/null; then
			alive=1
		else
			if [ "${PID_api:-}" = "$pid" ] && [ "$reported_api" = 0 ]; then
				reported_api=1
				printf '%s│%s %sapi exited%s — check the output above.\n' \
					"$C_DIM" "$C_OFF" "$C_ERR" "$C_OFF"
				note '  the portal is still running; it uses mock data until the engine exists.'
			fi
			if [ "${PID_web:-}" = "$pid" ] && [ "$reported_web" = 0 ]; then
				reported_web=1
				printf '%s│%s %sportal exited%s\n' "$C_DIM" "$C_OFF" "$C_ERR" "$C_OFF"
			fi
		fi
	done
	[ "$alive" = 0 ] && break
	sleep 1
done
