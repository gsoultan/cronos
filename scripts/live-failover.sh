#!/usr/bin/env bash
#
# Takes the definition store away, and puts it back.
#
# A database failover is an ordinary Tuesday and the classic reason a service
# has to be restarted afterwards: a pool full of connections to a server that no
# longer exists, and nothing that notices. What this asserts is that cronos
# needs no such restart — and, more interestingly, what keeps working while the
# store is gone.
#
#   1. store gone   → readiness says so, and a load balancer can route around it
#   2. store gone   → reports still render, because definitions live in memory
#                     and the warehouse is a different database entirely
#   3. store back   → everything recovers on its own, in about a second
#
# The second is the one worth having a check for. It is a real operational
# property — an outage of cronos's own database does not stop anybody reading a
# report — and it is the sort of thing that quietly stops being true the first
# time somebody reads a definition from the store on the render path.
#
#   ./scripts/live-failover.sh
#
# Needs go and podman (it stops and starts a container). Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

command -v podman >/dev/null 2>&1 || { echo "skipped: needs podman to stop a database"; exit 0; }

PORT=8784; API="http://localhost:$PORT"
PG=cronos-failover-test; PGPORT=55434
work=$(mktemp -d)
cleanup() {
	rm -rf "$work" || true
	[ -n "${srv:-}" ] && { kill -9 "$srv" 2>/dev/null || true; }
	podman rm -f "$PG" >/dev/null 2>&1 || true
	return 0
}
trap cleanup EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; [ -f "$work/log" ] && tail -15 "$work/log" >&2; exit 1; }

curl -s -o /dev/null --max-time 1 "$API/" 2>/dev/null && die "something is already listening on $PORT"

export CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef
go build -o bin/cronosd ./cmd/cronosd
go build -o bin/cronos-token ./cmd/cronos-token

podman rm -f "$PG" >/dev/null 2>&1 || true
podman run -d --name "$PG" -e POSTGRES_PASSWORD=cronos -e POSTGRES_USER=cronos \
	-e POSTGRES_DB=cronos -p "$PGPORT:5432" docker.io/library/postgres:16-alpine >/dev/null
# Over TCP, not the container's unix socket, and the -h is the whole point.
#
# The postgres image runs initdb against a temporary server started with
# listen_addresses='' — reachable on the socket, on no port — and then shuts it
# down before starting the real one. So a socket check goes ready, then not
# ready, then ready: the loop below breaks on the temporary server and the line
# after it lands in the shutdown and reports a database that never came up.
# Measured, not theorised: polling both during startup gives socket/tcp states
# of `..` `R.` `..` `RR`, and the container's log shows the fast shutdown
# between them.
#
# TCP is never ready until the real server is listening, which is also the only
# thing this check cares about — cronosd connects to localhost:$PGPORT.
for _ in $(seq 1 90); do podman exec "$PG" pg_isready -h 127.0.0.1 -U cronos >/dev/null 2>&1 && break; sleep 1; done
podman exec "$PG" pg_isready -h 127.0.0.1 -U cronos >/dev/null 2>&1 || die "the database never came up"

mkdir -p "$work/defs"
cp demo/definitions/*.yaml "$work/defs/"

env CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=postgres \
	CRONOS_STORE_DSN="postgres://cronos:cronos@localhost:$PGPORT/cronos?sslmode=disable" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef CRONOS_SEED=demo/seed.sql \
	CRONOS_ORG=acme CRONOS_PROJECT=finance \
	./bin/cronosd >"$work/log" 2>&1 & srv=$!
for _ in $(seq 1 80); do curl -sf "$API/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$API/v1/health" >/dev/null || die "no server"

TOK=$(./bin/cronos-token -audience portal -role admin -org acme -project finance -subject ada)
code() { curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$@"; }
authed() { code -H "Authorization: Bearer $TOK" "$API$1"; }
render() {
	curl -s --max-time 15 -X POST -H "Authorization: Bearer $TOK" \
		-H 'content-type: application/json' -d '{}' "$API/v1/reports/billing-summary" |
		python3 -c 'import json,sys
d = json.load(sys.stdin)
print(d["blocks"][0]["value"] if "blocks" in d else "refused")' 2>/dev/null || echo unreachable
}

# -- Before ------------------------------------------------------------------

[ "$(code "$API/v1/ready")" = 200 ] || die "not ready with the database up"
[ "$(authed /v1/runs)" = 200 ] || die "the run history does not answer with the database up"
want=$(render)
[ "$want" != refused ] && [ "$want" != unreachable ] || die "the report does not render with the database up"
ok "ready, and the report reads $want"

# -- 1. The store goes away --------------------------------------------------
#
# Readiness caches for five seconds — a probe is called often and hammering the
# database on each one is worse than a five-second lag — so the wait is that
# window, not an arbitrary sleep.

podman stop "$PG" >/dev/null 2>&1
down=0
for _ in $(seq 1 40); do
	[ "$(code "$API/v1/ready")" = 503 ] && { down=1; break; }
	sleep 0.5
done
[ "$down" = 1 ] ||
	die "readiness still says ok with the store gone — a load balancer would keep sending work here"
ok "readiness says 503, so an instance can be taken out of rotation"

curl -s --max-time 10 "$API/v1/ready" | grep -q '"store"' ||
	die "readiness does not name the store as the thing that is wrong"
ok "and names the store as the reason"

# The process is alive, so liveness must not restart it: a restart would not
# bring the database back and would lose whatever was in flight.
[ "$(code "$API/v1/health")" = 200 ] ||
	die "liveness failed with the store down — an orchestrator would restart a healthy process"
ok "liveness stays 200, so nothing restarts a process that is fine"

# -- 2. And reports keep rendering -------------------------------------------
#
# The property worth protecting. Definitions are held in memory and the
# warehouse is a different database, so an outage of cronos's own store does not
# stop anybody reading a report. That stops being true the first time somebody
# reads a definition from the store on the render path.

got=$(render)
[ "$got" = "$want" ] ||
	die "a report reads $got with the store down, and $want with it up"
ok "a report still renders, and reads the same $got"

embed=$(./bin/cronos-token -scope customer_id=c-1 -report billing-summary -org acme -project finance)
[ "$(code -X POST -H "Authorization: Bearer $embed" -H 'content-type: application/json' \
	-d '{}' "$API/v1/embed/reports/billing-summary")" = 200 ] ||
	die "an embedded reader cannot read during a store outage"
ok "and so does an embedded one, which is somebody else's customer"

# What genuinely needs the store says so rather than pretending.
[ "$(authed /v1/runs)" = 500 ] || die "the run history answered without a store"
ok "and the run history, which needs the store, refuses"

# -- 3. It comes back --------------------------------------------------------

podman start "$PG" >/dev/null 2>&1
for _ in $(seq 1 90); do podman exec "$PG" pg_isready -U cronos >/dev/null 2>&1 && break; sleep 1; done

back=0
for i in $(seq 1 60); do
	if [ "$(code "$API/v1/ready")" = 200 ] && [ "$(authed /v1/runs)" = 200 ]; then
		back=$i
		break
	fi
	sleep 0.5
done
[ "$back" != 0 ] ||
	die "it never recovered — a failover would need somebody to restart every instance"
ok "recovered on its own, without a restart"

printf '\n  \033[32mthe store can go and come back\033[0m — and reports never stopped\n\n'
