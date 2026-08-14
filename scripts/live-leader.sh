#!/usr/bin/env bash
#
# Runs three replicas with the scheduler armed on all of them, and counts.
#
# CRONOS_SCHEDULER had to be on for exactly one instance, which is a rule a
# deployment holds in its head rather than a thing the software enforces. Set it
# on two and every customer gets two copies of their statement. Forget it and
# nobody gets one. Both are quiet: the only party who notices a double delivery
# is the recipient, and the only party who notices no delivery is the recipient,
# a month later.
#
# So all three are armed here, which is what a Deployment with replicas: 3 does
# when somebody puts the flag in the pod template — the obvious thing to do, and
# the thing that used to triple every invoice.
#
# Then the leader is killed, and the check waits for one of the others to take
# over and deliver the next firing. That is the half that makes this worth
# doing: the scheduler stops being a single point of failure.
#
#   CRONOS_POSTGRES_DSN=postgres://... ./scripts/live-leader.sh
#
# Needs go, typst and a Postgres — leadership is an advisory lock, and SQLite
# has none because a SQLite deployment is one process. Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

DSN="${CRONOS_POSTGRES_DSN:-}"
[ -n "$DSN" ] || { echo "skipped: set CRONOS_POSTGRES_DSN — leadership needs a database that can arbitrate it"; exit 0; }

REPLICAS=3
BASE_PORT=8810
work=$(mktemp -d)
pids=""
cleanup() {
	for p in $pids; do kill -9 "$p" 2>/dev/null || true; done
	[ -n "${KEEP:-}" ] || rm -rf "$work" || true
	[ -n "${KEEP:-}" ] && echo "logs in $work" >&2
	return 0
}
trap cleanup EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; for i in $(seq 0 $((REPLICAS-1))); do
	[ -f "$work/log-$i" ] && { echo "--- replica $i ---" >&2; tail -8 "$work/log-$i" >&2; }
done; exit 1; }

go build -o bin/cronosd ./cmd/cronosd

# One schedule, every minute, delivering a file per firing. Counting files is
# how "exactly one replica fired" is measured — three leaders would leave three.
mkdir -p "$work/defs" "$work/out"
cat > "$work/defs/warehouse.yaml" <<YAML
apiVersion: cronos.dev/v1
kind: DataSource
metadata: {name: warehouse, title: Warehouse}
spec: {driver: sqlite, dsn: "file:$work/w.db"}
YAML
cat > "$work/defs/rows.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: rows, title: Rows}
spec:
  sources: [{ref: warehouse}]
  query: SELECT 1 AS n
  fields:
    - {name: n, type: number, role: measure, aggregate: sum, label: N}
YAML
cat > "$work/defs/report.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: nightly-report, title: Nightly report}
spec:
  dataset: rows
  outputs:
    # Paginated, because a burst attaches a document — a spreadsheet output is
    # refused with "renders spreadsheet, not a document".
    - name: pdf
      renderer: paginated
      page: {size: A4, orientation: portrait, margins: 18mm}
      # A table, because a paginated document is printed from one — a layout of
      # stats alone is refused with "has no table to print".
      layout: [{kind: table, title: Rows, columns: [n]}]
YAML
cat > "$work/defs/nightly.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Schedule
metadata: {name: nightly, title: Nightly}
spec:
  report: nightly-report
  output: pdf
  cron: "* * * * *"
  timezone: UTC
  deliver:
    - via: file
      to: everyone
      attach: {filename: "statement.pdf"}
YAML

# Its own schema, so this cannot collide with another test's tables — and its
# own delivery directory per replica, so "which one delivered" is answerable.
schema=cronos_leader_test
psql_do() { podman exec -i "${CRONOS_PG_CONTAINER:-cronos-pg}" psql "${CRONOS_PSQL_DSN:-$DSN}" -tAc "$1"; }
if [ -n "${CRONOS_PSQL:-}" ]; then
	psql_do() { $CRONOS_PSQL "${CRONOS_PSQL_DSN:-$DSN}" -tAc "$1"; }
fi
psql_do "DROP SCHEMA IF EXISTS $schema CASCADE; CREATE SCHEMA $schema" >/dev/null

scoped="$DSN&options=-csearch_path%3D$schema"

start_replica() {
	local i="$1" port=$((BASE_PORT + i))
	mkdir -p "$work/out-$i"
	env CRONOS_ADDR=":$port" \
		CRONOS_DEFINITIONS="$work/defs" \
		CRONOS_STORE_DRIVER=postgres CRONOS_STORE_DSN="$scoped" \
		CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
		CRONOS_DELIVERIES="$work/out-$i" \
		CRONOS_SCHEDULER=1 CRONOS_SCHEDULER_TICK=500ms \
		./bin/cronosd >"$work/log-$i" 2>&1 &
	eval "pid_$i=$!"
	pids="$pids $!"
	for _ in $(seq 1 60); do
		curl -sf "http://localhost:$port/v1/health" >/dev/null 2>&1 && return 0
		sleep 0.25
	done
	die "replica $i never came up"
}

delivered() { find "$work"/out-* -type f 2>/dev/null | wc -l | tr -d ' '; }
leaders() { grep -l "scheduling here now" "$work"/log-* 2>/dev/null | wc -l | tr -d ' '; }

# -- Three armed replicas, one scheduler --------------------------------------

for i in $(seq 0 $((REPLICAS - 1))); do start_replica "$i"; done
ok "$REPLICAS replicas, every one of them with CRONOS_SCHEDULER=1"

for _ in $(seq 1 40); do
	[ "$(leaders)" -ge 1 ] && break
	sleep 0.5
done
[ "$(leaders)" = "1" ] || die "$(leaders) replicas think they are scheduling — every customer would get $(leaders) copies"
ok "exactly one of them elected itself"

# A firing, and only one document for it.
for _ in $(seq 1 200); do
	[ "$(delivered)" -ge 1 ] && break
	sleep 0.5
done
[ "$(delivered)" -ge 1 ] || die "nobody fired at all in 100s — the leader is not scheduling"
sleep 3 # long enough for a second and third leader to have delivered too
n=$(delivered)
[ "$n" = "1" ] || die "$n documents for one firing — the schedule fired on more than one replica"
ok "one firing produced exactly one document"

# -- The leader dies ----------------------------------------------------------

led=$(basename "$(grep -l "scheduling here now" "$work"/log-* | head -1)")
which=${led#log-}
eval "victim=\$pid_$which"
# SIGKILL rather than SIGTERM: an orderly shutdown releases the lock, and the
# case worth proving is the one where nothing gets the chance to. Postgres ends
# the session when the socket closes, which is what frees the lock here.
kill -9 "$victim" 2>/dev/null || true
ok "killed replica $which, which was the one scheduling"

for _ in $(seq 1 60); do
	[ "$(leaders)" -ge 2 ] && break
	sleep 0.5
done
[ "$(leaders)" -ge 2 ] || die "no replica took over — the scheduler is still a single point of failure"
ok "another replica took over"

# And it actually fires, which is the point of taking over.
before=$(delivered)
for _ in $(seq 1 240); do
	[ "$(delivered)" -gt "$before" ] && break
	sleep 0.5
done
[ "$(delivered)" -gt "$before" ] || die "the new leader never delivered anything"
ok "and delivered the next firing"

printf '\n  \033[32mevery replica armed, one schedules\033[0m — and the schedule survives losing it\n\n'
