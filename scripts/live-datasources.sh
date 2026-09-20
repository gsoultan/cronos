#!/usr/bin/env bash
#
# More than one datasource per project, connected while the server is running.
#
# A deployment gets its first datasource from a YAML file somebody committed.
# It gets its second from the portal, which has a four-step wizard for
# connecting one — and until this check existed, that wizard published a
# definition the running process had no connection to. The catalogue listed the
# source, the Test button said "no such datasource", and every report reading it
# answered 500 until somebody restarted the server. So a deployment could have
# exactly the datasources it booted with, and adding one was a deploy.
#
# What is proved here is the whole claim, not the HTTP status of the publish:
#
#   - two sources connected through the API, each reaching its own database
#   - a report on the second one renders without a restart
#   - the first one keeps working while the second is added
#   - deleting a source closes it, rather than leaving a pool open to a
#     warehouse nobody has a definition for
#
# Each dataset asks SQLite which file it is attached to, so "both of these
# worked" cannot be satisfied by one database answering twice — which is what
# a deployment that falls back to CRONOS_DSN for everything would do.
#
#   ./scripts/live-datasources.sh
#
# Needs go. Leaves nothing behind.
set -euo pipefail

cd "$(dirname "$0")/.."

PORT=8796
API="http://127.0.0.1:$PORT"

work=$(mktemp -d)
cleanup() {
	[ -n "${server:-}" ] && { kill "$server" 2>/dev/null || true; }
	rm -rf "$work" || true
	return 0
}
trap 'cleanup 2>/dev/null' EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok() { printf '  \033[32mok\033[0m %s\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

authed() { curl -s -H "Authorization: Bearer $ADMIN" "${@:2}" "$API$1"; }
code() { curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $ADMIN" "${@:2}" "$API$1"; }
publish() {
	curl -s -o "$work/out" -w '%{http_code}' -X POST \
		-H "Authorization: Bearer $ADMIN" -H 'content-type: application/yaml' \
		--data-binary @"$1" "$API/v1/definitions"
}

# --- a deployment that starts with nothing ------------------------------------

say "A deployment with no datasources at all"
go build -o bin/cronosd ./cmd/cronosd || die "build"
mkdir -p "$work/defs"

CRONOS_ADDR=":$PORT" \
	CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=sqlite \
	CRONOS_STORE_DSN="file:$work/cronos.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_DRIVER=sqlite CRONOS_DSN="file:$work/dev.db" CRONOS_SEED=demo/seed.sql \
	./bin/cronosd >"$work/log" 2>&1 &
server=$!
for _ in $(seq 1 60); do
	curl -sf "$API/v1/health" >/dev/null 2>&1 && break
	sleep 0.25
done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/log"; die "cronos never came up"; }

ADMIN=$(curl -s -X POST -H 'content-type: application/json' \
	-d '{"email":"ops@acme.example","name":"Ada Rahayu","password":"a-password-they-chose","org":"acme","project":"finance"}' \
	"$API/v1/setup" |
	python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))')
[ -n "$ADMIN" ] || { cat "$work/log"; die "no session came back from /v1/setup"; }
ok "set up, and nothing is connected yet"

# The development path, which must survive all of this: with no sources
# defined, every dataset reads the configured database, so a demo shows a
# number without four YAML files first.
cat >"$work/dev.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Dataset
metadata:
  name: dev-rows
spec:
  sources:
    - ref: nothing-defined-this
  query: "SELECT count(*) AS n FROM customers"
  fields:
    - {name: n, type: integer, role: measure, aggregate: sum, label: Rows}
YAML
[ "$(publish "$work/dev.yaml")" = 200 ] || { cat "$work/out"; die "the dataset was refused"; }
cat >"$work/dev-report.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Report
metadata:
  name: dev-report
spec:
  dataset: dev-rows
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: stat
          label: Rows
          value: {field: n, aggregate: sum}
YAML
[ "$(publish "$work/dev-report.yaml")" = 200 ] || { cat "$work/out"; die "the report was refused"; }

rendered=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" \
	-H 'content-type: application/json' -d '{}' "$API/v1/reports/dev-report")
case "$rendered" in
*'"kind":"stat"'*) ok "and a dataset still reads the configured database, as it always has" ;;
*) die "the development path is broken: $rendered" ;;
esac

# --- two sources, connected through the API -----------------------------------

say "Connecting two databases, the way the portal does"
for which in one two; do
	cat >"$work/source-$which.yaml" <<YAML
apiVersion: cronos.dev/v1
kind: DataSource
metadata:
  name: warehouse-$which
  title: Warehouse $which
spec:
  driver: sqlite
  dsn: "file:$work/$which.db"
  limits:
    statementTimeout: 30s
    maxRows: 1000
YAML
	[ "$(publish "$work/source-$which.yaml")" = 200 ] ||
		{ cat "$work/out"; die "warehouse-$which could not be published"; }
done
ok "both published"

names=$(authed /v1/catalog |
	python3 -c 'import json,sys; print(",".join(sorted(s["name"] for s in json.load(sys.stdin)["sources"])))')
[ "$names" = "warehouse-one,warehouse-two" ] ||
	die "the catalogue lists $names"
ok "and the catalogue lists both: $names"

# The point of the whole check. A definition that is stored and not opened
# answers this with ok:false and "no such datasource" — which is what it did.
for which in one two; do
	answer=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" \
		"$API/v1/datasources/warehouse-$which/test")
	case "$answer" in
	*'"ok":true'*) ;;
	*) die "warehouse-$which is stored but not connected: $answer" ;;
	esac
done
ok "and the running process holds a connection to each, with no restart"

# --- and each dataset reaches its own -----------------------------------------

say "Each dataset reads its own database"
for which in one two; do
	cat >"$work/dataset-$which.yaml" <<YAML
apiVersion: cronos.dev/v1
kind: Dataset
metadata:
  name: where-$which
  title: Where $which
spec:
  sources:
    - ref: warehouse-$which
  # SQLite's own answer to "which file am I?", so two sources cannot both be
  # satisfied by one database answering twice.
  query: "SELECT file AS path FROM pragma_database_list() WHERE name = 'main'"
  fields:
    - {name: path, type: string, role: dimension, label: Database}
YAML
	[ "$(publish "$work/dataset-$which.yaml")" = 200 ] ||
		{ cat "$work/out"; die "dataset where-$which was refused"; }

	cat >"$work/report-$which.yaml" <<YAML
apiVersion: cronos.dev/v1
kind: Report
metadata:
  name: which-$which
  title: Which $which
spec:
  dataset: where-$which
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: table
          title: Database
          columns: [path]
YAML
	[ "$(publish "$work/report-$which.yaml")" = 200 ] ||
		{ cat "$work/out"; die "report which-$which was refused"; }
done

for which in one two; do
	rendered=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" \
		-H 'content-type: application/json' -d '{}' "$API/v1/reports/which-$which")
	case "$rendered" in
	*"$work/$which.db"*) ;;
	*) die "which-$which did not read $work/$which.db — it answered: $rendered" ;;
	esac
done
ok "warehouse-one reads one.db and warehouse-two reads two.db"

# And the configured database stops being the answer for everybody.
#
# The other half of the development path, and the half that matters once a
# project has warehouses: a dataset naming a source that is not there is an
# error. Reading CRONOS_DSN instead would mean a typo in `ref:` silently
# serving the development database's rows under a production report's name.
failed=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" \
	-H 'content-type: application/json' -d '{}' "$API/v1/reports/dev-report")
case "$failed" in
*'"error"'*) ok "and a dataset naming a source that is not there now fails, rather than reading it" ;;
*) die "a dataset naming nothing still read the configured database: $failed" ;;
esac

# --- a third, while the first two are serving ---------------------------------

say "A third source, added while the other two are serving"
cat >"$work/source-three.yaml" <<YAML
apiVersion: cronos.dev/v1
kind: DataSource
metadata:
  name: warehouse-three
spec:
  driver: sqlite
  dsn: "file:$work/three.db"
YAML
[ "$(publish "$work/source-three.yaml")" = 200 ] || die "a third source was refused"

answer=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" "$API/v1/datasources/warehouse-three/test")
case "$answer" in
*'"ok":true'*) ok "connected" ;;
*) die "warehouse-three is stored but not connected: $answer" ;;
esac

rendered=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" \
	-H 'content-type: application/json' -d '{}' "$API/v1/reports/which-one")
case "$rendered" in
*"$work/one.db"*) ok "and the report on the first one still reads the first one" ;;
*) die "adding a source moved an existing report: $rendered" ;;
esac

# --- and deleting one closes it -----------------------------------------------

say "Deleting a source"
# Refused while something reads it, which is the existing guard and is checked
# here because the delete below would otherwise prove nothing about ordering.
[ "$(code /v1/definitions/DataSource/warehouse-two -X DELETE)" = 409 ] ||
	die "a source still read by a dataset could be deleted"
ok "refused while a dataset still reads it"

[ "$(code /v1/definitions/Report/which-two -X DELETE)" = 204 ] || die "the report would not delete"
[ "$(code /v1/definitions/Dataset/where-two -X DELETE)" = 204 ] || die "the dataset would not delete"
[ "$(code /v1/definitions/DataSource/warehouse-two -X DELETE)" = 204 ] || die "the source would not delete"

answer=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" "$API/v1/datasources/warehouse-two/test")
case "$answer" in
*'no such datasource'*) ok "and the connection to it is gone, with no restart" ;;
*) die "a deleted source is still connected: $answer" ;;
esac

rendered=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" \
	-H 'content-type: application/json' -d '{}' "$API/v1/reports/which-one")
case "$rendered" in
*"$work/one.db"*) ok "and the other reports are untouched" ;;
*) die "deleting one source broke another: $rendered" ;;
esac

say "Everything a project connects, it can use — without a restart"
