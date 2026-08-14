#!/usr/bin/env bash
#
# Interrupts a burst and counts what arrived.
#
# cronos is a report scheduler, so the work it exists to do happens in a
# goroutine rather than in an HTTP handler — and that is the path a rolling
# deploy lands on. Six in the morning on the first of the month is when the
# monthly statements burst runs and also when somebody ships.
#
# Half a customer list receiving a document while the other half does not is the
# worst state to be left in: nobody can tell from outside which half, and the
# run record that could say is written at the end.
#
# The failure this pins was invisible to every unit test in the repository and
# visible from here in one line. schedule.Service.Start held a WaitGroup over
# its in-flight runs and blocked on it when cancelled — a correct drain that
# nothing could reach, because boot launched these goroutines, kept no handle on
# them, cancelled, and returned. The runtime then tore down the goroutine doing
# the waiting. The drain written to let a burst finish was what killed it.
#
#   ./scripts/live-drain.sh
#
# Needs go and typst. Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

PORT=8794; API="http://localhost:$PORT"
# Enough that the burst is still going when it is interrupted, and few enough
# that it finishes inside the drain. Each recipient is one typeset document;
# forty of them went by in under a fifth of a second, which is a check that
# passes because the race was never run.
RECIPIENTS=800

work=$(mktemp -d)
trap 'rm -rf "$work"; [ -n "${srv:-}" ] && kill -9 "$srv" 2>/dev/null; true' EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; [ -f "$work/log" ] && tail -30 "$work/log" >&2; exit 1; }

go build -o bin/cronosd ./cmd/cronosd

# A warehouse with enough customers to fan out over.
mkdir -p "$work/defs" "$work/out"
{
  echo 'CREATE TABLE customers (id TEXT PRIMARY KEY, name TEXT);'
  echo 'INSERT INTO customers VALUES'
  for i in $(seq 1 $RECIPIENTS); do
    printf "('c-%03d','Customer %d')%s\n" "$i" "$i" "$([ "$i" -lt $RECIPIENTS ] && echo , || echo ';')"
  done
} > "$work/seed.sql"

cat > "$work/defs/warehouse.yaml" <<YAML
apiVersion: cronos.dev/v1
kind: DataSource
metadata: {name: warehouse, title: Warehouse}
spec: {driver: sqlite, dsn: "file:$work/w.db"}
YAML

# Two datasets: one to fan out over, one scoped to the customer being billed.
cat > "$work/defs/customers.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: customers, title: Customers}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, name FROM customers ORDER BY id
  fields:
    - {name: id, type: string, role: dimension, label: Customer}
    - {name: name, type: string, role: dimension, label: Name}
YAML

cat > "$work/defs/one-customer.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: one-customer, title: One customer}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, name FROM customers WHERE id = {{ .params.customer_id }}
  params:
    - {name: customer_id, type: string, required: true, label: Customer}
  fields:
    - {name: id, type: string, role: dimension, label: Customer}
    - {name: name, type: string, role: dimension, label: Name}
YAML

# One row per customer, bound by the burst. A paginated output, because that is
# what a burst attaches — the same shape as the demo's monthly statements, and
# slow enough per recipient that the interruption below lands mid-delivery
# rather than by luck.
cat > "$work/defs/statement.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: statement, title: Statement}
spec:
  dataset: one-customer
  outputs:
    - name: pdf
      renderer: paginated
      page: {size: A4, orientation: portrait, margins: 18mm}
      layout:
        - kind: table
          title: Customer
          columns: [id, name]
YAML

cat > "$work/defs/nightly.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Schedule
metadata: {name: nightly, title: Nightly}
spec:
  report: statement
  output: pdf
  # Every minute, so the check does not wait for a clock.
  cron: "* * * * *"
  timezone: UTC
  burst:
    over:
      dataset: customers
    bind:
      customer_id: "{{ .row.id }}"
    # Typesetting is the cost here, so keep several going: a burst that
    # rendered one document at a time would be a check about typst's speed.
    concurrency: 4
  deliver:
    - via: file
      to: "{{ .row.id }}"
      attach: {filename: "statement-{{ .row.id }}.pdf"}
YAML

CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
  CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
  CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
  CRONOS_SEED="$work/seed.sql" CRONOS_SEED_SOURCE=warehouse \
  CRONOS_DELIVERIES="$work/out" \
  CRONOS_SCHEDULER=1 CRONOS_SCHEDULER_TICK=200ms \
  ./bin/cronosd >"$work/log" 2>&1 & srv=$!

for _ in $(seq 1 60); do curl -sf "$API/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$API/v1/health" >/dev/null || die "no server"
ok "a server with $RECIPIENTS recipients and a schedule every minute"

# Wait for the burst to be under way — some files written, not all. Polled
# tightly, because the window is the whole point: sleeping a quarter of a second
# between looks is how a check ends up interrupting a burst that has finished.
started=0
for _ in $(seq 1 2000); do
  n=$(find "$work/out" -type f 2>/dev/null | wc -l | tr -d ' ')
  if [ "$n" -gt 0 ]; then started=$n; break; fi
  sleep 0.05
done
[ "$started" -gt 0 ] || die "the burst never started — the scheduler did not fire"
[ "$started" -lt "$RECIPIENTS" ] || die "the burst finished before it could be interrupted"
ok "the burst is under way ($started of $RECIPIENTS delivered)"

# SIGTERM, which is what an orchestrator sends.
kill -TERM "$srv"

# Wait for the process to go. A drain that never completes is its own failure,
# and this bound is above the drain's own.
gone=0
for _ in $(seq 1 120); do
  kill -0 "$srv" 2>/dev/null || { gone=1; break; }
  sleep 0.25
done
[ "$gone" = 1 ] || die "the server did not stop within 30s of SIGTERM"
ok "the server stopped"

delivered=$(find "$work/out" -type f | wc -l | tr -d ' ')
if [ "$delivered" -lt "$RECIPIENTS" ]; then
  die "the burst was cut short: $delivered of $RECIPIENTS delivered — $(( RECIPIENTS - delivered )) customers got nothing"
fi
ok "every one of the $RECIPIENTS recipients was delivered to"

grep -q "scheduler draining" "$work/log" || die "the scheduler never drained"
ok "the scheduler drained rather than being killed"

printf '\n  \033[32mthe burst finished\033[0m — SIGTERM mid-delivery cost nobody a document\n\n'
