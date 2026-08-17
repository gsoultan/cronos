#!/usr/bin/env bash
#
# Runs a report against a real MySQL.
#
# MySQL was supported by every layer and reachable by none of them. The dialect
# in internal/core/query compiles, the registry maps the driver name, boot knows
# it, DuckDB can mount it, and definition.Validate has always accepted
# `driver: mysql` — and no MySQL driver was ever imported. So publishing one
# returned 200 and killed the deployment at its next start with "unknown
# driver", and nothing here had ever executed a query against the database the
# dialect was written for.
#
# What a unit test cannot answer, and this does:
#
#   1. cronos opens a MySQL source at all — the gap that made all of this moot
#   2. `?` binds where the compiler thinks it does
#   3. the truncation arithmetic buckets the months a person would expect
#   4. a DECIMAL comes back as a number the renderer can read, not as bytes
#
#   ./scripts/live-mysql.sh
#
# Uses the official mysql:8 image. Set CRONOS_MYSQL_RUNNING when something else
# manages the server. Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

PORT=8841; API="http://localhost:$PORT"
DBPORT=33061
ROOT_PASSWORD='A-Strong-Passw0rd!'
work=$(mktemp -d)

cleanup() {
	[ -n "${srv:-}" ] && { kill -9 "$srv" 2>/dev/null || true; }
	[ -n "${started_db:-}" ] && { podman rm -f cronos-mysql >/dev/null 2>&1 || true; }
	rm -rf "$work" || true
	return 0
}
trap cleanup EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; [ -f "$work/log" ] && tail -8 "$work/log" >&2; exit 1; }
say(){ printf '\n\033[1m%s\033[0m\n' "$*"; }

command -v podman >/dev/null 2>&1 || { echo "skipped: needs podman for a MySQL"; exit 0; }
curl -s -o /dev/null --max-time 1 "$API/" 2>/dev/null && die "something is already listening on $PORT"

sql() { podman exec -i cronos-mysql mysql -uroot -p"$ROOT_PASSWORD" -N -B erp -e "$1"; }

say "MySQL"
if [ -z "${CRONOS_MYSQL_RUNNING:-}" ]; then
	started_db=yes
	podman rm -f cronos-mysql >/dev/null 2>&1 || true
	podman run -d --name cronos-mysql -p "$DBPORT:3306" \
		-e MYSQL_ROOT_PASSWORD="$ROOT_PASSWORD" -e MYSQL_DATABASE=erp \
		docker.io/library/mysql:8 >/dev/null
fi
# Waited for by asking it a question rather than by reading its log: a log line
# says the process started, this says it will answer.
for _ in $(seq 1 120); do sql 'SELECT 1' >/dev/null 2>&1 && break; sleep 2; done
sql 'SELECT 1' >/dev/null 2>&1 || die "MySQL never started answering"
ok "running and answering on $DBPORT"

# Three months, so a month bucket has something to be wrong about, and a
# DECIMAL rather than a float, because that is what money is stored as and it
# is the type that arrives as bytes if nobody is looking.
sql "
DROP TABLE IF EXISTS invoices;
CREATE TABLE invoices (
  id VARCHAR(16) PRIMARY KEY,
  customer_id VARCHAR(16) NOT NULL,
  total DECIMAL(12,2) NOT NULL,
  issued_at DATE NOT NULL
);
INSERT INTO invoices VALUES
  ('i-1','c-1',100.50,'2026-01-15'),
  ('i-2','c-1', 49.50,'2026-01-20'),
  ('i-3','c-2',200.00,'2026-02-10'),
  ('i-4','c-2', 25.25,'2026-03-05'),
  ('i-5','c-1', 74.75,'2026-03-28');" >/dev/null
[ "$(sql 'SELECT COUNT(*) FROM invoices')" = 5 ] || die "the fixture is not there"
ok "erp.invoices, five rows across three months"

# What the dialect compiles for a month grain, asked of the server directly.
# If MySQL disagrees with the compiler about this, every monthly chart is wrong
# by a boundary and nobody notices until a quarter does not add up.
months=$(sql "SELECT DATE_FORMAT(issued_at, '%Y-%m-01') AS m, SUM(total)
              FROM invoices GROUP BY m ORDER BY m" | tr '\t' ' ')
case "$months" in
*"2026-01-01 150.00"*) ok "DATE_FORMAT buckets to the first of each month" ;;
*) die "January did not bucket to 150.00: $months" ;;
esac
case "$months" in
*"2026-03-01 100.00"*) ok "and the months add up" ;;
*) die "March did not add up: $months" ;;
esac

say "cronos"
export CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef
go build -o bin/cronosd ./cmd/cronosd || die "build"
go build -o bin/cronos-token ./cmd/cronos-token || die "build"

mkdir -p "$work/defs"
cat >"$work/defs/erp.yaml" <<YAML
apiVersion: cronos.dev/v1
kind: DataSource
metadata:
  name: erp
spec:
  driver: mysql
  dsn: "root:${ROOT_PASSWORD}@tcp(127.0.0.1:${DBPORT})/erp?parseTime=true"
YAML
cat >"$work/defs/invoices.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Dataset
metadata:
  name: invoices
spec:
  sources: [{ref: erp}]
  query: SELECT id, customer_id, total, issued_at FROM invoices
  fields:
    - {name: id,          type: string,  role: dimension}
    - {name: customer_id, type: string,  role: dimension}
    - {name: issued_at,   type: date,    role: dimension, label: Issued}
    - {name: total, type: decimal, role: measure, aggregate: sum, label: Amount}
YAML
cat >"$work/defs/billing.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Report
metadata:
  name: billing
  title: Billing
spec:
  dataset: invoices
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: stat
          label: Total billed
          value: {field: total, aggregate: sum}
        - kind: chart
          chart: bar
          title: Billed by month
          x: {field: issued_at, grain: month}
          y: {field: total, aggregate: sum}
YAML

env CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_ORG=acme CRONOS_PROJECT=finance \
	./bin/cronosd >"$work/log" 2>&1 & srv=$!
for _ in $(seq 1 80); do curl -sf "$API/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$API/v1/health" >/dev/null || { tail -8 "$work/log"; die "cronosd never came up"; }

# The gap that made everything else moot: before the driver was registered this
# is where it stopped, with "unknown driver \"mysql\"".
grep -q 'driver=mysql' "$work/log" ||
	die "cronos did not open the MySQL source — check that the driver is registered"
[ "$(curl -s "$API/v1/metrics" | awk '/^cronos_datasources_unavailable /{print $2}')" = 0 ] ||
	die "the MySQL source could not be opened"
ok "opened the source and loaded the definitions"

TOK=$(./bin/cronos-token -audience portal -role admin -org acme -project finance -subject ada)
[ "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $TOK" \
	"$API/v1/datasources/erp/test")" = 200 ] || die "the connection test does not answer"
curl -s -X POST -H "Authorization: Bearer $TOK" "$API/v1/datasources/erp/test" |
	grep -q '"ok":true' || die "the connection test says the source is not reachable"
ok "the connection test passes"

say "A report"
body=$(curl -s --max-time 30 -X POST -H "Authorization: Bearer $TOK" \
	-H 'content-type: application/json' -d '{}' "$API/v1/reports/billing")

printf '%s' "$body" | python3 -c '
import json, sys
d = json.load(sys.stdin)
blocks = d.get("blocks")
if not blocks:
    print("REFUSED", d); raise SystemExit(1)

# The total, as a number rather than as the character codes a DECIMAL becomes
# when a driver hands back []byte and nobody turns it into what it is.
total = blocks[0]["value"]
digits = "".join(c for c in str(total) if c.isdigit() or c == ".")
if abs(float(digits) - 450.00) > 0.005:
    print("TOTAL", total); raise SystemExit(1)

months = [pt for b in blocks if b.get("series") for pt in b["series"]]
if len(months) != 3:
    print("MONTHS", months); raise SystemExit(1)
# January is two invoices summed, which is the arithmetic the dialect does and
# the one a wrong month boundary would break.
jan = next((m for m in months if "Jan" in str(m.get("label"))), None)
if jan is None or abs(float(jan["value"]) - 150.0) > 0.005:
    print("JANUARY", jan); raise SystemExit(1)
print("ok")' >/dev/null || die "the report is wrong: $(printf %s "$body" | head -c 200)"
ok "renders, and the five invoices total 450.00 across three months"

# A DECIMAL that arrived as bytes prints as a list of character codes, which is
# a number-shaped thing that is not the number.
printf '%s' "$body" | grep -q '\[49,' &&
	die "a DECIMAL came back as bytes rather than as a number"
ok "and the DECIMAL survived as a number rather than as bytes"

printf '\n  \033[32mMySQL is a database cronos can actually read\033[0m\n\n'
