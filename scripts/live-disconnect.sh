#!/usr/bin/env bash
#
# Hangs up on a slow report and asks the warehouse whether it noticed.
#
# A reporting tool reads databases somebody else operates, and the promise it
# makes to their DBA is that it stops when nobody is waiting. A browser tab
# closed on a slow dashboard must not leave a query running to its own timeout:
# on a warehouse with a connection limit, a page somebody refreshes four times
# is four queries where there should be one.
#
# Nothing but a real database can answer this. The chain is long — request
# context, handler, run service, executor, database/sql, driver, protocol — and
# every unit test along it owns both ends. Postgres will say plainly, in
# pg_stat_activity, whether the statement is still there.
#
#   CRONOS_POSTGRES_DSN=postgres://... ./scripts/live-disconnect.sh
#
# Needs go and a Postgres. Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

DSN="${CRONOS_POSTGRES_DSN:-}"
[ -n "$DSN" ] || { echo "skipped: set CRONOS_POSTGRES_DSN to a database this may use"; exit 0; }

PORT=8793; API="http://localhost:$PORT"
work=$(mktemp -d)
trap 'rm -rf "$work"; [ -n "${srv:-}" ] && kill -9 "$srv" 2>/dev/null; true' EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; [ -f "$work/log" ] && tail -20 "$work/log" >&2; exit 1; }

if curl -s -o /dev/null --max-time 1 "$API/" 2>/dev/null; then
	die "something is already listening on $PORT"
fi

go build -o bin/cronosd ./cmd/cronosd

# A report whose only block sleeps. pg_sleep is the honest way to make a query
# slow: no data volume to generate, and the server is genuinely busy in a way
# pg_stat_activity reports.
mkdir -p "$work/defs"
cat > "$work/defs/warehouse.yaml" <<YAML
apiVersion: cronos.dev/v1
kind: DataSource
metadata: {name: warehouse, title: Warehouse}
spec:
  driver: postgres
  dsn: "$DSN"
  limits:
    # Well above the wait below, so the timeout is never what ends the query.
    # What is under test is the disconnect, not the statement timeout.
    statementTimeout: 120s
YAML

cat > "$work/defs/slow.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: slow, title: Slow}
spec:
  sources: [{ref: warehouse}]
  query: SELECT 'cronos-disconnect-probe'::text AS marker, pg_sleep(60) IS NULL AS done
  fields:
    - {name: marker, type: string, role: dimension, label: Marker}
    - {name: done, type: bool, role: dimension, label: Done}
YAML

cat > "$work/defs/report.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: slow-report, title: Slow report}
spec:
  dataset: slow
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: table
          title: Slow
          columns: [marker]
YAML

CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	./bin/cronosd >"$work/log" 2>&1 & srv=$!

for _ in $(seq 1 60); do curl -sf "$API/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$API/v1/health" >/dev/null || die "no server"

# The store is fresh, so this is a first run.
curl -sf -o /dev/null -X POST -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","name":"Ada","password":"an-administrators-password","org":"Acme","project":"Finance"}' \
	"$API/v1/setup" || die "setup"
token=$(curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","password":"an-administrators-password"}' |
	sed 's/.*"token":"//; s/".*//')
[ -n "$token" ] || die "no session"
ok "a server, and a report whose one block sleeps for a minute"

# Ask Postgres directly, using its own view of what it is running.
#
# CRONOS_PSQL so a laptop without a local client can point at one inside a
# container — "podman exec -i cronos-pg psql", say. CI has the client.
#
# CRONOS_PSQL_DSN because the client may not reach the database by the same
# address the server does: a container publishing 5432 on the host's 55432 is
# two addresses for one database, and asking the wrong one reports an idle
# server every time. Same value in CI, where both run on the host.
PSQL="${CRONOS_PSQL:-psql}"
PSQL_DSN="${CRONOS_PSQL_DSN:-$DSN}"
running() {
	$PSQL "$PSQL_DSN" -tAc \
		"SELECT count(*) FROM pg_stat_activity
		  WHERE query LIKE '%cronos-disconnect-probe%'
		    AND query NOT LIKE '%pg_stat_activity%'
		    AND state <> 'idle'" 2>/dev/null | tr -d ' '
}
$PSQL "$PSQL_DSN" -tAc 'SELECT 1' >/dev/null 2>&1 ||
	die "cannot reach the database with '$PSQL' — set CRONOS_PSQL if the client is elsewhere"

# Start the render and leave somebody waiting on it.
#
# In the background rather than with --max-time, because the check has to see
# the query running *before* the hang-up. Without that half it would pass just
# as happily against a render that never reached the database at all — which is
# the way a check like this usually rots.
curl -s -o /dev/null -X POST -H "Authorization: Bearer $token" \
	-H 'content-type: application/json' -d '{}' \
	"$API/v1/reports/slow-report" & client=$!

started=0
for _ in $(seq 1 60); do
	n=$(running)
	if [ "${n:-0}" != "0" ]; then started=$n; break; fi
	sleep 0.25
done
[ "$started" != "0" ] || {
	kill -9 "$client" 2>/dev/null
	die "the query never reached the warehouse — this check would prove nothing"
}
ok "the warehouse is running the query while somebody waits"

# The hang-up. Killing curl closes the connection, which is what a closed tab
# does and what a load balancer does when it gives up on a client.
kill -9 "$client" 2>/dev/null || true
wait "$client" 2>/dev/null || true

# Postgres cancels asynchronously, so a single look is a race.
for _ in $(seq 1 60); do
	[ "$(running)" = "0" ] && break
	sleep 0.25
done

left=$(running)
[ "$left" = "0" ] || die "$left query still running on the warehouse after the client hung up"
ok "the warehouse stopped when the client did"

printf '\n  \033[32mnobody is waiting, so nothing is running\033[0m\n\n'
