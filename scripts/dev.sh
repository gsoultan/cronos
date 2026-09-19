#!/usr/bin/env bash
#
#> Runs cronos for local development: the Go API and the portal dev server side
#> by side, with prefixed output and one Ctrl-C that stops both. There is
#> nothing to set up — it seeds an administrator and the demo reports, and
#> prints what to sign in with.
#>
#>   scripts/dev.sh              both, connected, already set up
#>   scripts/dev.sh --web        portal only
#>   scripts/dev.sh --api        API only
#>   scripts/dev.sh --setup      both, and set the deployment up by hand
#>   scripts/dev.sh --samples    portal on sample data, talking to nothing
#>
#>   sign in  dev@cronos.local / cronos-dev-password
#>
#> Ports come from CRONOS_API_PORT / CRONOS_WEB_PORT, the account from
#> CRONOS_DEV_EMAIL / CRONOS_DEV_PASSWORD. Delete .dev/ to start over.
#
# The usage above is marked rather than addressed by line number: --help used to
# print lines 2 to 10, so adding one to it silently truncated the last.
#
# Connected is the default, and it did not used to be. This script started both
# halves and never told the portal where the API was, so it ran on sample data
# beside a server nobody was talking to — and every part of cronos that needs an
# account was invisible in development: signing in, signing out, the first-run
# setup, invitations, second factors, the people list, the deployment tab.
#
# That is not a small gap in a workflow. It is how a two-factor wizard that
# accepted any six digits and a device list that was invented in the browser both
# survived: anybody running this command saw the sample portal, where they were
# never shown.
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
CONNECTED=1
SEED=1
case "${1:-}" in
	--web|--web-only) RUN_API=0 ;;
	--api|--api-only) RUN_WEB=0 ;;
	# The first-run wizard, on purpose. It is a real page with real consequences
	# — the organisation it names is the one the deployment adopts — and the only
	# way to see it is a deployment that has not been set up. Kept for that, and
	# not the default: nobody should have to fill in a form to open a report.
	--setup|--first-run) SEED=0 ;;
	# Sample data, which is what the browser suites exercise and what makes the
	# interface workable before a server exists. Worth keeping and worth not
	# being the default.
	--samples|--sample) CONNECTED=0 ;;
	'') ;;
	-h|--help) sed -n 's/^#> \{0,1\}//p' "$0"; exit 0 ;;
	*) echo "unknown option: $1 (try --help)" >&2; exit 2 ;;
esac

# Nothing to seed without a server of our own to seed it into.
[ "$RUN_API" = 0 ] && SEED=0
[ "$CONNECTED" = 0 ] && SEED=0

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
		[ -n "$owner" ] && { kill "$owner" 2>/dev/null || true; }
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

# A database, so there are accounts to sign in as.
#
# Without one the store is file-backed: definitions work, and there is nobody to
# be — no sign-in, no setup, no people, no second factor. SQLite in a gitignored
# directory, so a fresh clone gets a fresh deployment.
DEV_DIR="$ROOT/.dev"
mkdir -p "$DEV_DIR"

# One answer for both halves of this script: the seeder below and the API it is
# seeding for have to be looking at the same database, and two places reading
# CRONOS_STORE_DSN with two defaults is how they would stop.
STORE_DRIVER="${CRONOS_STORE_DRIVER:-sqlite}"
STORE_DSN="${CRONOS_STORE_DSN:-file:$DEV_DIR/cronos.db}"

DEV_EMAIL="${CRONOS_DEV_EMAIL:-dev@cronos.local}"
DEV_PASSWORD="${CRONOS_DEV_PASSWORD:-cronos-dev-password}"
# What says this deployment is ours to seed, written once we have.
OURS="$DEV_DIR/dev-account"

# Not a store somebody else is keeping their work in. A developer who points
# CRONOS_STORE_DSN at their own database has their own accounts in it, and this
# script is not the thing that should be writing to it.
if [ "$SEED" = 1 ] && { [ -n "${CRONOS_STORE_DSN:-}" ] || [ "$STORE_DRIVER" != sqlite ]; }; then
	SEED=0
fi

# The same command an operator runs on a real install, against the store the API
# is about to open, before it opens it. Nothing here relaxes the server: the
# portal signs this account in through the ordinary password path, and the only
# reason the password can be written down is that it is a SQLite file in a
# gitignored directory on one laptop.
seed_account() {
	local out guard=-if-empty
	# Ours to repair if we seeded this store before — the grant is idempotent
	# and the password is left alone, so a deployment somebody half broke comes
	# back on the next start. Otherwise -if-empty: a store that already has
	# accounts is somebody's work, including the one --setup produced, and
	# cronosd adopts the organisation that store names. A second administrator
	# in default/default would sign in to a project with no definitions in it,
	# which looks exactly like a broken deployment.
	[ -f "$OURS" ] && guard=''
	out="$(printf '%s\n' "$DEV_PASSWORD" | (cd "$ROOT" && go run ./cmd/cronos-user \
		${guard} \
		-email "$DEV_EMAIL" -name "${CRONOS_DEV_NAME:-Dev}" \
		-org "${CRONOS_ORG:-default}" -project "${CRONOS_PROJECT:-default}" \
		-role admin -platform \
		-driver "$STORE_DRIVER" -dsn "$STORE_DSN") 2>&1)" || {
		printf '%s\n' "$out" >&2
		fail "could not prepare $DEV_EMAIL — \`scripts/dev.sh --setup\` sets the deployment up in the browser instead"
	}
	case "$out" in
		# It reported what it did, and the banner is about to tell somebody how
		# to sign in. Either this is the account it just made, or this store has
		# accounts of its own and the banner must not claim otherwise.
		*created*) printf '%s\n' "$DEV_EMAIL" > "$OURS" ;;
		*) [ -f "$OURS" ] || SEED=0 ;;
	esac
}

if [ "$SEED" = 1 ]; then
	note "preparing ${DEV_EMAIL}…"
	seed_account
fi

printf '\n'
note "cronos dev"
[ "$RUN_API" = 1 ] && note "  api    http://localhost:$API_PORT"
[ "$RUN_WEB" = 1 ] && note "  portal http://localhost:$WEB_PORT"
# Every line here is a claim about what is running, so each one is guarded by
# what actually decided it. "sample data — no server, no sign-in" used to print
# for --api and --web too, where all three words were wrong.
if [ "$CONNECTED" = 0 ]; then
	note "  sample data — no server, no sign-in"
elif [ "$RUN_API" = 0 ]; then
	note "  the API is yours to run — this portal talks to http://localhost:$API_PORT"
else
	if [ "$SEED" = 1 ]; then
		note "  sign in  $DEV_EMAIL / $DEV_PASSWORD"
		note "  reports  demo/definitions over demo/seed.sql, already published"
	elif [ -f "$OURS" ] || [ -n "${CRONOS_STORE_DSN:-}" ]; then
		note "  sign in with the account this deployment already has"
	else
		note "  first run — the portal will ask you to set the deployment up"
	fi
	note "  accounts in .dev/cronos.db — delete it to start over"
fi
printf '\n'

if [ "$RUN_API" = 1 ]; then
	# The demo definitions over the demo seed, so the first report someone
	# opens has real numbers in it rather than an empty state.
	CRONOS_ADDR=":$API_PORT" \
	CRONOS_SIGNING_KEY="${CRONOS_SIGNING_KEY:-development-key-at-least-32-bytes-long}" \
	CRONOS_DEFINITIONS="${CRONOS_DEFINITIONS:-demo/definitions}" \
	CRONOS_SEED="${CRONOS_SEED:-demo/seed.sql}" \
	CRONOS_ORIGINS="${CRONOS_ORIGINS:-http://localhost:$WEB_PORT}" \
	CRONOS_STORE_DRIVER="$STORE_DRIVER" \
	CRONOS_STORE_DSN="$STORE_DSN" \
		start api "$C_API" "$ROOT" go run ./cmd/cronosd
fi
if [ "$RUN_WEB" = 1 ]; then
	# Handed to bun, not run on its own, and not `bun run vite` — that wrapper
	# survives long enough to print an exit-code complaint every time you Ctrl-C.
	#
	# node_modules/.bin/vite is a symlink to a file whose shebang is
	# `#!/usr/bin/env node`, so executing it needs node on PATH. Preflight has
	# only ever asked for go and bun, and on a machine with bun but no node this
	# script printed its banner, the ports, where the accounts live — and then
	# `env: node: No such file or directory` and a dead portal. Every line above
	# was a promise the next line broke. Passing the file to bun runs it under
	# bun's runtime and ignores the shebang, so the two things this script checks
	# for are the two things it needs.
	# The one line that was missing. CRONOS_ORIGINS below has always allowed the
	# portal to call the API; nothing ever told the portal to.
	if [ "$CONNECTED" = 1 ]; then
		VITE_CRONOS_API="http://localhost:$API_PORT" \
			start web "$C_WEB" "$PORTAL" bun "$PORTAL/node_modules/.bin/vite" \
			--port "$WEB_PORT" --strictPort
	else
		start web "$C_WEB" "$PORTAL" bun "$PORTAL/node_modules/.bin/vite" \
			--port "$WEB_PORT" --strictPort
	fi
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
				# What the portal actually does now, which is not what this
				# said. It said the portal falls back to sample data — true
				# when it talked to nothing, and false since it was pointed at
				# this API. A connected portal whose server has gone shows
				# failed requests on every page, and somebody reading this line
				# would go looking for the bug in the portal.
				if [ "$CONNECTED" = 1 ]; then
					note '  the portal is still running, and every page it loads will fail:'
					note "  it is pointed at http://localhost:$API_PORT, which is now nothing."
				else
					note '  the portal is still running on sample data, which needs no API.'
				fi
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
