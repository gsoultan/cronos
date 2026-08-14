#!/usr/bin/env bash
#
# Runs the restore drill from docs/deploying.md, exactly as written.
#
# A backup nobody has restored is a hypothesis. The drill has been in the docs
# for a while and nothing ever ran it, which is the same shape as every other
# gap this repository has found: a procedure that is correct on the day it is
# written and drifts silently afterwards, because the only thing that would
# notice is somebody having a bad morning.
#
# What drifts is the schema. A migration that adds a NOT NULL column with no
# default, or a table an older dump does not carry, breaks a restore and nothing
# else — the forward path keeps working perfectly, and the failure waits for the
# day the backup is the only copy left.
#
# The last step is the one that matters. A database that restores and a product
# that serves are different claims, and only the second is what anybody is
# asking about. So this renders a report out of the restored database and checks
# the number in it.
#
#   CRONOS_POSTGRES_DSN=postgres://... ./scripts/live-restore.sh
#
# Needs go, a Postgres and pg_dump/psql. Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

DSN="${CRONOS_POSTGRES_DSN:-}"
[ -n "$DSN" ] || { echo "skipped: set CRONOS_POSTGRES_DSN — restoring needs a database to restore into"; exit 0; }

LIVE_PORT=8788; LIVE="http://localhost:$LIVE_PORT"
RESTORED_PORT=8789; RESTORED="http://localhost:$RESTORED_PORT"

work=$(mktemp -d)
cleanup() {
	rm -rf "$work" || true
	[ -n "${live:-}" ] && { kill -9 "$live" 2>/dev/null || true; }
	[ -n "${restored:-}" ] && { kill -9 "$restored" 2>/dev/null || true; }
	return 0
}
trap cleanup EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; for f in "$work"/log-*; do
	[ -f "$f" ] && { echo "--- $(basename "$f") ---" >&2; tail -12 "$f" >&2; }
done; exit 1; }

for p in "$LIVE_PORT" "$RESTORED_PORT"; do
	curl -s -o /dev/null --max-time 1 "http://localhost:$p/" 2>/dev/null &&
		die "something is already listening on $p"
done

PSQL="${CRONOS_PSQL:-psql}"
PGDUMP="${CRONOS_PGDUMP:-pg_dump}"
PSQL_DSN="${CRONOS_PSQL_DSN:-$DSN}"

go build -o bin/cronosd ./cmd/cronosd

live_schema=cronos_restore_live
dead_schema=cronos_restore_target
$PSQL "$PSQL_DSN" -tAc "
	DROP SCHEMA IF EXISTS $live_schema CASCADE; CREATE SCHEMA $live_schema;
	DROP SCHEMA IF EXISTS $dead_schema CASCADE" >/dev/null 2>&1

# -- A deployment with something in it ----------------------------------------

mkdir -p "$work/defs" "$work/empty"
cat > "$work/defs/warehouse.yaml" <<YAML
apiVersion: cronos.dev/v1
kind: DataSource
metadata: {name: warehouse, title: Warehouse}
spec: {driver: sqlite, dsn: "file:$work/w.db"}
YAML
cat > "$work/defs/invoices.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: invoices, title: Invoices}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, total FROM invoices
  fields:
    - {name: id, type: string, role: dimension, label: Invoice}
    - {name: total, type: decimal, role: measure, aggregate: sum, label: Amount}
YAML
cat > "$work/defs/billing.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: billing, title: Billing}
spec:
  dataset: invoices
  outputs:
    - name: interactive
      renderer: interactive
      layout: [{kind: stat, label: Total billed, value: {field: total, aggregate: sum}}]
YAML
cat > "$work/seed.sql" <<'SQL'
CREATE TABLE invoices (id TEXT PRIMARY KEY, total REAL);
INSERT INTO invoices VALUES ('i-1', 1500.25), ('i-2', 2499.75), ('i-3', 1000.00);
SQL

env CRONOS_ADDR=":$LIVE_PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=postgres CRONOS_STORE_DSN="$DSN&options=-csearch_path%3D$live_schema" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_SEED="$work/seed.sql" CRONOS_SEED_SOURCE=warehouse \
	./bin/cronosd >"$work/log-live" 2>&1 & live=$!
for _ in $(seq 1 80); do curl -sf "$LIVE/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$LIVE/v1/health" >/dev/null || die "the live deployment never came up"

tok=$(curl -s "$LIVE/v1/setup" -X POST -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","name":"Ada","password":"an-administrators-password","org":"Acme","project":"Finance"}' |
	python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')
[ -n "$tok" ] || die "could not set the deployment up"

# The number the restored copy has to produce. Read from the live one rather
# than hardcoded: what matters is that the two agree, not what the sum happens
# to be.
want=$(curl -sf -X POST -H "Authorization: Bearer $tok" -H 'content-type: application/json' \
	-d '{}' "$LIVE/v1/reports/billing" |
	python3 -c 'import json,sys; print(json.load(sys.stdin)["blocks"][0]["value"])')
[ -n "$want" ] || die "the live deployment could not render the report"
ok "a deployment with an account, four definitions and a report reading $want"

# -- 1. Back it up, the way the docs say --------------------------------------

$PGDUMP "$PSQL_DSN" --schema="$live_schema" --no-owner --no-privileges > "$work/backup.sql" 2>/dev/null ||
	die "pg_dump failed"
[ -s "$work/backup.sql" ] || die "the backup is empty"
ok "backed up ($(wc -c < "$work/backup.sql" | tr -d ' ') bytes)"

# The original is gone. Restoring beside a database that still has everything
# proves nothing — the check has to be able to fail.
$PSQL "$PSQL_DSN" -tAc "DROP SCHEMA $live_schema CASCADE" >/dev/null 2>&1
kill -9 "$live" 2>/dev/null || true; wait "$live" 2>/dev/null || true; live=""
ok "and dropped the original, so what follows is the backup or nothing"

# -- 2. Restore into an empty database ----------------------------------------

# The dump creates its own schema, so nothing is created here — and the name is
# rewritten to the new one, which is what restoring into a different database
# does for real.
#
# ON_ERROR_STOP, because psql without it reports success after failing every
# statement. The first version of this check restored nothing and said it had:
# the server then found an empty store and seeded itself from the definitions
# directory, which looks exactly like a working restore from outside.
sed "s/$live_schema/$dead_schema/g" "$work/backup.sql" |
	$PSQL "$PSQL_DSN" -v ON_ERROR_STOP=1 >/dev/null 2>"$work/restore.err" ||
	{ tail -5 "$work/restore.err" >&2; die "restoring the backup failed"; }
ok "restored into an empty schema"

# -- 3. It must reach ready ---------------------------------------------------
#
# Which proves the schema is the one this build knows. A migration that added a
# NOT NULL column with no default would fail here, and nowhere else.

# No seed and no definitions directory. Both would hide a failed restore: an
# empty store adopts the directory and looks like a working one, and the seed
# would rebuild the warehouse the report reads. What serves here has to have
# come out of the backup.
env CRONOS_ADDR=":$RESTORED_PORT" CRONOS_DEFINITIONS="$work/empty" \
	CRONOS_STORE_DRIVER=postgres CRONOS_STORE_DSN="$DSN&options=-csearch_path%3D$dead_schema" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	./bin/cronosd >"$work/log-restored" 2>&1 & restored=$!
for _ in $(seq 1 80); do curl -sf "$RESTORED/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$RESTORED/v1/health" >/dev/null || die "a cronosd on the restored database would not start"

curl -sf "$RESTORED/v1/ready" | grep -q '"status":"ok"' ||
	die "the restored database is not a schema this build knows"
ok "a cronosd on it reaches ready"

# -- 4. And it serves ---------------------------------------------------------
#
# The step that matters. A database that restores and a product that serves are
# different claims, and only the second is what anybody is asking about.

# The same account, because the users came back with everything else. Signing in
# proves the credential survived the round trip, which is what somebody locked
# out at three in the morning is actually asking.
back=$(curl -s "$RESTORED/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","password":"an-administrators-password"}' |
	python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))')
[ -n "$back" ] || die "the account did not survive the restore — nobody can sign in"
ok "the account came back and can sign in"

got=$(curl -sf -X POST -H "Authorization: Bearer $back" -H 'content-type: application/json' \
	-d '{}' "$RESTORED/v1/reports/billing" |
	python3 -c 'import json,sys; print(json.load(sys.stdin)["blocks"][0]["value"])' 2>/dev/null || true)
[ -n "$got" ] || die "the restored deployment could not render the report — the definitions did not come back whole"
[ "$got" = "$want" ] || die "the restored report reads $got, the original read $want"
ok "and renders the report, reading $got — the same as before"

printf '\n  \033[32mthe backup is a backup\033[0m — restored, ready, signed in and serving\n\n'
