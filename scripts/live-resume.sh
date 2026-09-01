#!/usr/bin/env bash
#
# Leaves a burst partly delivered, resumes it, and counts what each customer got.
#
# A burst is one document per recipient and it can stop halfway: a deploy, an
# expired grace, a mail relay refusing for ten minutes. What that leaves is the
# state nobody can reconcile — some customers have their statement and some do
# not — and the only recovery was to run the whole thing again, which sends a
# second copy to everybody who already had one. On an invoice that is worse than
# the gap it was fixing.
#
# The partial state is made deliberately rather than by racing a real burst. The
# file channel creates a directory per recipient, so a plain file where that
# directory should go makes exactly those deliveries fail and no others. An
# earlier version of this check killed the server mid-burst and counted files,
# and was wrong twice over: the file count lagged the deliveries badly enough to
# report nineteen when the records said sixty, and the file channel writes one
# name per customer, so a second copy overwrites the first and a duplicate can
# never appear in a directory listing.
#
# So both numbers come from cronos_deliveries, which is the record of what was
# sent and what somebody reconciling an incident would actually read.
#
#   CRONOS_POSTGRES_DSN=postgres://... ./scripts/live-resume.sh
#
# Needs go, typst and a Postgres — a resume reads what was delivered from run
# history, which a file-backed deployment does not have. Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

DSN="${CRONOS_POSTGRES_DSN:-}"
[ -n "$DSN" ] || { echo "skipped: set CRONOS_POSTGRES_DSN — a resume reads what was delivered from run history"; exit 0; }

PORT=8799; API="http://localhost:$PORT"
RECIPIENTS=12
# The ones whose delivery is made to fail, spread through the list so the
# resume has to pick them out rather than take a suffix.
BLOCKED="c-002 c-005 c-009 c-012"

work=$(mktemp -d)
cleanup() {
	rm -rf "$work" || true
	[ -n "${srv:-}" ] && { kill -9 "$srv" 2>/dev/null || true; }
	return 0
}
trap cleanup EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; [ -f "$work/log" ] && tail -20 "$work/log" >&2; exit 1; }

curl -s -o /dev/null --max-time 1 "$API/" 2>/dev/null && die "something is already listening on $PORT"

go build -o bin/cronosd ./cmd/cronosd

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
      layout: [{kind: table, title: Customer, columns: [id, name]}]
YAML
# The first of January, so nothing fires by itself and every delivery in this
# check is one the check asked for.
cat > "$work/defs/nightly.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Schedule
metadata: {name: nightly, title: Nightly}
spec:
  report: statement
  output: pdf
  cron: "0 6 1 1 *"
  timezone: UTC
  burst:
    over: {dataset: customers}
    bind: {customer_id: "{{ .row.id }}"}
  deliver:
    - via: file
      to: "{{ .row.id }}"
      attach: {filename: "statement.pdf"}
  onFailure:
    retries: 0
YAML

schema=cronos_resume_test
PSQL="${CRONOS_PSQL:-psql}"
PSQL_DSN="${CRONOS_PSQL_DSN:-$DSN}"

# Scoped in the query rather than with a SET: `psql -tAc "SET ...; SELECT"`
# prints the SET's own output too, which lands in the number.
#
# The failure is reported rather than discarded. `psql … 2>/dev/null | tr` under
# `set -e` and `pipefail` ends the script the moment psql is unhappy — silently,
# because the failure is a pipeline rather than a command and its complaint has
# just been thrown away. This exited 1 after one green tick with not a word
# about why, which is the third time this shape has cost an afternoon here; see
# the note on `freeport` in live-portal-2fa.sh for the second.
q() {
	local out
	if ! out=$($PSQL "$PSQL_DSN" -tAc "$1" 2>&1); then
		printf '  \033[31mFAILED\033[0m the database would not answer:\n    %s\n    %s\n' \
			"$(printf %s "$1" | tr -s ' \n' ' ')" "$out" >&2
		exit 1
	fi
	printf %s "$out" | tr -d ' \n'
}
reached() {
	q "SELECT count(DISTINCT destination) FROM $schema.cronos_deliveries
		WHERE status = 'delivered'"
}
twice() {
	q "SELECT count(*) FROM (
		SELECT destination FROM $schema.cronos_deliveries WHERE status = 'delivered'
		GROUP BY destination, channel HAVING count(*) > 1
	) AS x"
}

# Notices are dropped — a cascade drop lists every table it takes, and that
# noise buries whatever this check actually reports — but a failure is not.
# This is the first thing that talks to the database, so when it was
# `>/dev/null 2>&1` under `set -e` a wrong DSN ended the run here with no
# output at all, before a single line had been printed.
if ! setup_out=$($PSQL "$PSQL_DSN" -tAc \
	"DROP SCHEMA IF EXISTS $schema CASCADE; CREATE SCHEMA $schema" 2>&1); then
	printf '  \033[31mFAILED\033[0m could not prepare %s:\n    %s\n' "$schema" "$setup_out" >&2
	exit 1
fi
scoped="$DSN&options=-csearch_path%3D$schema"

env CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=postgres CRONOS_STORE_DSN="$scoped" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_SEED="$work/seed.sql" CRONOS_SEED_SOURCE=warehouse \
	CRONOS_DELIVERIES="$work/out" CRONOS_SCHEDULER=1 \
	./bin/cronosd >"$work/log" 2>&1 & srv=$!
for _ in $(seq 1 80); do curl -sf "$API/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$API/v1/health" >/dev/null || die "no server"

tok=$(curl -s "$API/v1/setup" -X POST -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","name":"Ada","password":"an-administrators-password","org":"Acme","project":"Finance"}' |
	grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)
[ -n "$tok" ] || die "could not set the deployment up"
ok "a server with $RECIPIENTS recipients"

# -- 1. A burst that reaches most of them -------------------------------------
#
# A plain file where the channel wants a directory, so those four deliveries
# fail and no others do.
for id in $BLOCKED; do : > "$work/out/$id"; done

code=$(curl -s -o "$work/run.json" -w '%{http_code}' -X POST \
	-H "Authorization: Bearer $tok" "$API/v1/schedules/nightly/run")
[ "$code" = "202" ] || { cat "$work/run.json"; die "running the schedule answered $code"; }

blocked_count=$(printf '%s\n' $BLOCKED | wc -l | tr -d ' ')
want=$((RECIPIENTS - blocked_count))
got=$(reached)
[ "$got" = "$want" ] ||
	die "$got of $RECIPIENTS delivered, wanted $want — the partial state is not what this check assumes"
ok "$got of $RECIPIENTS delivered; $blocked_count customers have no statement"

# -- 2. Resume it -------------------------------------------------------------

for id in $BLOCKED; do rm -f "$work/out/$id"; done

runs=$(curl -s -o "$work/runs.json" -w '%{http_code}' -H "Authorization: Bearer $tok" "$API/v1/runs?limit=1")
[ "$runs" = "200" ] || { cat "$work/runs.json"; die "reading the run history answered $runs"; }
# Parsed rather than pattern-matched. The response is pretty-printed, so a
# line-oriented sed returns the opening brace and a grep for `"id":"` misses the
# space after the colon — both of which this check did in turn.
run=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print((d if isinstance(d,list) else d.get("runs",[]))[0]["id"])' "$work/runs.json" 2>/dev/null || true)
[ -n "$run" ] || { cat "$work/runs.json"; die "the partial burst left no run record"; }

code=$(curl -s -o "$work/resume.json" -w '%{http_code}' -X POST \
	-H "Authorization: Bearer $tok" "$API/v1/runs/$run/resume")
[ "$code" = "202" ] || { cat "$work/resume.json"; die "resume answered $code"; }
ok "resumed run $run"

# -- 3. Exactly one each ------------------------------------------------------
#
# Both directions. A resume that skipped everybody would leave the blocked four
# at nothing; one that skipped nobody would send the other eight a second copy.

for _ in $(seq 1 240); do
	[ "$(reached)" = "$RECIPIENTS" ] && break
	sleep 0.25
done

got=$(reached)
[ "$got" = "$RECIPIENTS" ] ||
	die "$got of $RECIPIENTS customers have a statement — the resume skipped people who needed one"
ok "all $RECIPIENTS customers were delivered to"

doubled=$(twice)
[ "${doubled:-0}" = "0" ] ||
	die "$doubled customers were sent two — the resume re-sent to people who already had theirs"
ok "and not one of them was sent twice"

grep -q "already had theirs" "$work/log" || die "the resume did not report what it skipped"
ok "and the log says how many already had theirs"

printf '\n  \033[32ma partly delivered burst resumes\033[0m — nobody missed out, nobody got two\n\n'
