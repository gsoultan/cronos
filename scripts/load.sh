#!/usr/bin/env bash
# Measures cronos under load, against a real cronosd and a real database.
#
# Tier 3 — pool sizing, burst concurrency, caching — was deferred on the
# grounds that it needs numbers. This is what produces them. Nothing here
# asserts a threshold: a benchmark that fails a build teaches people to raise
# the threshold, and the point is to see what the shape is.
#
# The corpus is generated rather than committed. A thousand customers with five
# invoices each is a megabyte of SQL that would sit in the repository being
# scrolled past, and it is three lines of awk.
set -euo pipefail
cd "$(dirname "$0")/.."

# The warehouse under test. The demo's is shared-cache in-memory SQLite, which
# serialises readers with a table lock of its own — so measuring cronos's
# concurrency against it measures SQLite's, and the first run of this reported
# "it serialises somewhere" about a fixture. Postgres is what a deployment
# reads, and is what these numbers should be about.
WAREHOUSE="${WAREHOUSE:-postgres}"
PGDSN="${PGDSN:-postgres://cronos:cronos@localhost:5433/cronos?sslmode=disable}"

CUSTOMERS="${CUSTOMERS:-1000}"
REQUESTS="${REQUESTS:-300}"
PORT="${PORT:-8783}"
# KEEP=1 leaves the corpus, the definitions and the server's log behind. A run
# that reports something surprising is a run whose log has just been deleted,
# which is how "every request failed with 500" took three goes to look at.
WORK="$(mktemp -d)"
if [ -n "${KEEP:-}" ]; then
  trap 'kill %1 2>/dev/null || true; echo; echo "kept: $WORK"' EXIT
else
  trap 'kill %1 2>/dev/null || true; rm -rf "$WORK"' EXIT
fi

# Braced, because the ellipsis that follows is not ASCII: bash reads
# `$WAREHOUSE…` as one identifier and `set -u` then aborts on a variable
# nobody named. It survived by never being run in CI.
echo "generating a corpus of $CUSTOMERS customers for ${WAREHOUSE}…"
{
  cat <<'SQL'
CREATE TABLE customers (id TEXT PRIMARY KEY, name TEXT, city TEXT);
-- DATE rather than TEXT, which is what a warehouse anybody would recognise
-- uses. SQLite is typeless and does not care; Postgres does, and a dataset
-- declaring `type: date` over a text column fails there on date_trunc — a
-- portability seam worth knowing about and not worth measuring around.
CREATE TABLE invoices (
  id TEXT PRIMARY KEY, customer_id TEXT, issued_at DATE,
  currency TEXT, total REAL, status TEXT
);
CREATE TABLE shipments (
  id TEXT PRIMARY KEY, customer_id TEXT, dispatched_at DATE,
  region TEXT, weight_kg REAL, status TEXT
);
SQL
  awk -v n="$CUSTOMERS" '
    BEGIN {
      cities["0"]="Rotterdam"; cities["1"]="Gdansk"; cities["2"]="Bristol";
      cities["3"]="Lyon"; cities["4"]="Porto";
      status["0"]="paid"; status["1"]="sent"; status["2"]="overdue"; status["3"]="draft";
      print "INSERT INTO customers VALUES";
      for (i = 1; i <= n; i++) {
        printf "%s(\047c-%d\047,\047Customer %d\047,\047%s\047)", (i>1 ? "," : ""), i, i, cities[i % 5];
      }
      print ";";
      print "INSERT INTO invoices VALUES";
      k = 0;
      for (i = 1; i <= n; i++) {
        for (j = 1; j <= 5; j++) {
          k++;
          printf "%s(\047i-%d\047,\047c-%d\047,\0472026-%02d-%02d\047,\047EUR\047,%d.50,\047%s\047)", \
            (k>1 ? "," : ""), k, i, (j % 12) + 1, (i % 28) + 1, (i * j) % 40000 + 100, status[k % 4];
        }
      }
      print ";";
    }'
  # Indexes, because the shape being measured is cronos and not a table scan.
  echo "CREATE INDEX invoices_by_customer ON invoices (customer_id);"
} > "$WORK/seed.sql"

echo "building…"
go build -o bin/cronosd ./cmd/cronosd
go build -o bin/cronos-token ./cmd/cronos-token

# The definitions, with the warehouse pointed wherever this run is measuring.
cp -r demo/definitions "$WORK/definitions"

if [ "$WAREHOUSE" = "postgres" ]; then
  if ! command -v psql >/dev/null 2>&1 && ! command -v podman >/dev/null 2>&1; then
    echo "postgres mode needs psql or podman; re-run with WAREHOUSE=sqlite" >&2
    exit 1
  fi
  echo "loading the corpus into postgres…"
  # The corpus is portable SQL already: TEXT, REAL and plain inserts.
  {
    echo "DROP TABLE IF EXISTS invoices, customers, shipments CASCADE;"
    cat "$WORK/seed.sql"
  } | podman exec -i cronos-pg psql -q -U cronos -d cronos >/dev/null

  # Quoted, so ${secret:…} reaches the file rather than being expanded by the
  # shell into nothing — which produced a datasource with no dsn and a server
  # that refused to start, reported here as a connection refused.
  cat > "$WORK/definitions/warehouse.yaml" <<'YAML'

apiVersion: cronos.dev/v1
kind: DataSource
metadata:
  name: warehouse
  title: Load warehouse
spec:
  driver: postgres
  dsn: "${secret:pg_dsn}"
  limits:
    statementTimeout: 30s
    maxRows: 1000000
YAML
  export CRONOS_SECRET_PG_DSN="$PGDSN"
  unset CRONOS_SEED
else
  export CRONOS_SEED="$WORK/seed.sql"
fi

# The burst's worker count, so "is eight the right default" is a question this
# can answer rather than assert.
if [ -n "${BURST_WORKERS:-}" ]; then
  SCHED="$WORK/definitions/monthly-statements.yaml"
  if LC_ALL=C grep -q "concurrency:" "$SCHED"; then
    # Replaced, not added. Inserting a second `concurrency` into the same map
    # is a duplicate key, which the decoder refuses — so the definition failed
    # to load, the server did not start, and four runs reported the same
    # number because none of them ran.
    LC_ALL=C sed -i.bak "s/^\( *\)concurrency: [0-9]*/\1concurrency: ${BURST_WORKERS}/" "$SCHED"
  else
    LC_ALL=C sed -i.bak "s/^    bind:/    concurrency: ${BURST_WORKERS}\\
    bind:/" "$SCHED"
  fi
  rm -f "$SCHED.bak"
  echo "burst workers: $(LC_ALL=C grep -c 'concurrency:' "$SCHED") setting at ${BURST_WORKERS}"
fi

export CRONOS_SIGNING_KEY="${CRONOS_SIGNING_KEY:-development-key-at-least-32-bytes-long}"
export CRONOS_DEFINITIONS="$WORK/definitions"
export CRONOS_ORG=acme CRONOS_PROJECT=finance
export CRONOS_ADDR=":$PORT"
export CRONOS_SCHEDULER=1
export CRONOS_DELIVERIES="$WORK/deliveries"
export CRONOS_STORE_DRIVER=sqlite
export CRONOS_STORE_DSN="file:$WORK/store.db"
export CRONOS_ADMIN_KEY="admin-key-at-least-32-bytes-long-ok"
# Off, because a thousand audit lines a second is measuring the terminal.
export CRONOS_AUDIT=off

./bin/cronosd > "$WORK/server.log" 2>&1 &
for _ in $(seq 1 60); do
  curl -sf -o /dev/null "http://localhost:$PORT/v1/health" && break
  sleep 0.5
done

# One token per virtual reader, because that is what load is. A single token
# driven at sixty-four in flight is one reader in a loop, which is exactly what
# the render limit exists to stop — the first run of this measured the limiter
# and reported it as the server.
READERS="${READERS:-64}"
: > "$WORK/tokens"
for i in $(seq 1 "$READERS"); do
  ./bin/cronos-token -audience portal -role editor -org acme -project finance \
    -subject "reader-$i" >> "$WORK/tokens"
done
TOKEN="$(head -1 "$WORK/tokens")"

# What the process looked like before any of it, so the numbers after have
# something to be a change from. Read from the API's own exposition, because
# CRONOS_METRICS_ADDR moves /v1/metrics to a listener of its own — setting it
# here silently 404'd the section below that reads what the server counted.
runtime_now() {
  curl -sf "http://localhost:$PORT/v1/metrics" 2>/dev/null |
    awk '/^cronos_goroutines /{g=$2} /^cronos_heap_bytes /{h=$2} END{printf "%s goroutines, %.0f MB heap", g, h/1048576}'
}
before="$(runtime_now)"

API="http://localhost:$PORT" TOKEN="$TOKEN" TOKENS="$WORK/tokens" ADMIN="$CRONOS_ADMIN_KEY" \
  CUSTOMERS="$CUSTOMERS" REQUESTS="$REQUESTS" WORK="$WORK" \
  node scripts/load-check.mjs

# A count that went up and stayed up is a leak, and it is the failure mode a
# throughput number cannot show: the run that measures fastest is often the one
# that left the most behind. Read after a pause, because a request still being
# served is a goroutine that has not finished rather than one that will not.
sleep 5
echo
echo "--- what the process did to itself ---"
printf '  before  %s\n  after   %s\n' "$before" "$(runtime_now)"

echo
echo "--- what the server said about itself ---"
grep -E "level=(WARN|ERROR)" "$WORK/server.log" | head -10 || echo "  nothing at warn or above"
