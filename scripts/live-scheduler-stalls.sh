#!/usr/bin/env bash
#
# Stops a scheduler without stopping the server, and checks somebody would know.
#
# This is the failure with no signal. Every alert cronos documented counts
# things that happened — runs, deliveries, failures — and a scheduler that is
# not running produces none of them. Zero failures is what a perfect night looks
# like and also what a dead loop looks like, and no alert written against a
# counter can tell them apart.
#
# The process stays up throughout. Health is 200, readiness is ok, every request
# is served. That is what makes this the hardest thing here to see and the most
# expensive to miss: the first person to know is a customer who did not get an
# invoice.
#
# So the check asks the question an operator's alerting would:
#
#   1. armed and going round     — seconds_since_tick stays small
#   2. armed and stuck           — seconds_since_tick grows without bound
#   3. armed nothing at all      — cronos_scheduler_armed is 0
#
#   ./scripts/live-scheduler-stalls.sh
#
# Needs go. Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

PORT=8796; API="http://localhost:$PORT"
METRICS_PORT=9796; METRICS="http://127.0.0.1:$METRICS_PORT"

work=$(mktemp -d)
cleanup() {
	rm -rf "$work" || true
	[ -n "${srv:-}" ] && { kill -9 "$srv" 2>/dev/null || true; }
	return 0
}
trap cleanup EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; [ -f "$work/log" ] && tail -20 "$work/log" >&2; exit 1; }

for p in "$PORT" "$METRICS_PORT"; do
	curl -s -o /dev/null --max-time 1 "http://localhost:$p/" 2>/dev/null &&
		die "something is already listening on $p"
done

go build -o bin/cronosd ./cmd/cronosd

mkdir -p "$work/defs"
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
    - name: interactive
      renderer: interactive
      layout: [{kind: stat, label: N, value: {field: n, aggregate: sum}}]
YAML
# Due in the past by the time it arms, so "overdue" is a real number rather
# than a wait. A schedule armed forward never looks late, which is correct and
# is not what is under test.
cat > "$work/defs/nightly.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Schedule
metadata: {name: nightly, title: Nightly}
spec:
  report: nightly-report
  output: interactive
  cron: "*/5 * * * *"
  timezone: UTC
  deliver: [{via: file, to: somebody}]
YAML

# Extra environment comes in as arguments, so `env` rather than a bare "$@":
# a variable assignment expanded from "$@" is a command name, not an assignment.
start() {
	env CRONOS_ADDR=":$PORT" CRONOS_METRICS_ADDR="127.0.0.1:$METRICS_PORT" \
		CRONOS_DEFINITIONS="$work/defs" \
		CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
		CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
		CRONOS_DELIVERIES="$work/out" \
		"$@" ./bin/cronosd >"$work/log" 2>&1 & srv=$!
	for _ in $(seq 1 60); do curl -sf "$API/v1/health" >/dev/null 2>&1 && return 0; sleep 0.25; done
	die "no server"
}

# The gauge an operator would alert on.
metric() {
	curl -sf "$METRICS/metrics" | sed -n "s/^$1[^ ]* \(.*\)$/\1/p" | head -1
}

# -- 1. Armed and going round -------------------------------------------------

start CRONOS_SCHEDULER=1 CRONOS_SCHEDULER_TICK=200ms
[ "$(metric cronos_scheduler_armed)" = "1" ] || die "an armed process does not say so"
ok "the process says it is running schedules"

# Three seconds of a two-hundred-millisecond tick. The number has to stay near
# zero across that, which is the half that says the *loop* is recording its
# passes rather than only the start-up doing it once — a scheduler that recorded
# one pass at boot and none after would climb exactly like a stalled one, and
# every alert would be right for the wrong reason.
sleep 3
since=$(metric cronos_scheduler_seconds_since_tick)
[ -n "$since" ] || die "no seconds_since_tick at all"
if [ "${since%.*}" -gt 1 ]; then
	die "after three seconds of a 200ms tick the loop reports ${since}s — only the start-up pass is being recorded, so a working scheduler is indistinguishable from a stalled one"
fi
ok "a loop going round stays up to date (${since}s after three seconds of ticks)"

armed=$(metric cronos_schedules_armed)
[ "${armed%.*}" -ge 1 ] || die "nothing armed, so nothing is under test"
ok "$armed schedule armed"

# -- 2. Time since the last pass is real, and it climbs --------------------
#
# The alert is "seconds_since_tick above a few times the tick", so what has to
# be true is that the number climbs in real time between passes and resets when
# one happens. A loop that stops is then just a loop whose gap never ends.
#
# Freezing the process with SIGSTOP was the first idea and does not work: the
# metrics listener freezes with it, so the scrape is answered after the thaw and
# by then the loop has already caught up. The gauge is computed at scrape time —
# which is what lets it report a stall at all, and what makes a stall
# unobservable from outside a stopped process. A unit test holds that half, with
# a clock it controls.

kill -9 "$srv" 2>/dev/null || true
wait "$srv" 2>/dev/null || true

# A tick long enough that the gap between passes is measurable.
rm -f "$work/c.db"
start CRONOS_SCHEDULER=1 CRONOS_SCHEDULER_TICK=60s

first=$(metric cronos_scheduler_seconds_since_tick)
[ -n "$first" ] || die "no seconds_since_tick at all"
[ "${first%.*}" -le 1 ] || die "a scheduler that just started reports ${first}s"
ok "a pass at startup, so a fresh scheduler is not reported as stuck"

sleep 4
climbed=$(metric cronos_scheduler_seconds_since_tick)
if [ "${climbed%.*}" -lt 3 ]; then
	die "four seconds without a pass still reports ${climbed}s — the gauge does not measure elapsed time, so no alert on it could ever fire"
fi
ok "four seconds later it reads ${climbed}s, so an alert above the tick would fire"

# And the process is entirely healthy throughout, which is the whole point:
# nothing else here would say a word.
[ "$(curl -s -o /dev/null -w '%{http_code}' "$API/v1/health")" = "200" ] ||
	die "health was not 200 — this check is about a process that looks fine"
[ "$(curl -s "$API/v1/ready" | grep -c '"status":"ok"')" = "1" ] ||
	die "readiness was not ok"
ok "meanwhile health is 200 and readiness is ok, which is why nothing else catches this"

# Overdue is its own signal: what should have fired and has not.
overdue=$(metric cronos_schedule_overdue_seconds)
[ -n "$overdue" ] || die "no overdue gauge"
ok "and the business-level number is there too (${overdue}s past due)"

kill -9 "$srv" 2>/dev/null || true
wait "$srv" 2>/dev/null || true
srv=""

# -- 3. Armed nothing at all --------------------------------------------------
#
# The state an operator reaches on their first install, because the scheduler is
# off by default. Every counter stays at zero, which is indistinguishable from a
# quiet week — so the only thing that can say is the gauge.
rm -f "$work/c.db"
start
[ "$(metric cronos_scheduler_armed)" = "0" ] ||
	die "a process running no schedules claims to be scheduling"
ok "a process with the scheduler off reports 0, so a fleet reporting all zeroes is visible"

printf '\n  \033[32ma scheduler that stops is visible\033[0m — from a process that looks perfectly healthy\n\n'
