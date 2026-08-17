#!/usr/bin/env bash
#
# One editor's typo took the whole deployment down.
#
# A schedule's timezone was checked for being non-empty and never for existing.
# So "Europe/Berln" published with a 200 and the running instance carried on
# perfectly well — a schedule is parsed when the process starts, not when it is
# stored. Then the next restart found a schedule that would not arm and refused
# to start at all.
#
# That is an outage for every project in the deployment, triggered by anybody
# who can edit, landing days later on a deploy that looks like it broke
# something. And with the API down, the only way to remove the definition was a
# prompt on the database — the shape of every unrecoverable failure, where
# fixing the broken thing needs the broken thing.
#
# Both halves are checked here, because either alone is not enough:
#
#   1. the typo is refused where the person who typed it can still see it
#   2. a store that already holds one still starts, still serves, and can be
#      repaired through the API
#
#   ./scripts/live-typo.sh
#
# Needs go. Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

PORT=8821; API="http://localhost:$PORT"
work=$(mktemp -d)
cleanup() {
	[ -n "${srv:-}" ] && { kill -9 "$srv" 2>/dev/null || true; }
	rm -rf "$work" || true
	return 0
}
trap cleanup EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; [ -f "$work/log" ] && tail -8 "$work/log" >&2; exit 1; }
say(){ printf '\n\033[1m%s\033[0m\n' "$*"; }

curl -s -o /dev/null --max-time 1 "$API/" 2>/dev/null && die "something is already listening on $PORT"

export CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef
go build -o bin/cronosd ./cmd/cronosd
go build -o bin/cronos-token ./cmd/cronos-token

mkdir -p "$work/defs"
cp demo/definitions/*.yaml "$work/defs/"

start() {
	env CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
		CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
		CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
		CRONOS_SEED=demo/seed.sql CRONOS_ORG=acme CRONOS_PROJECT=finance \
		CRONOS_SCHEDULER=1 "$@" \
		./bin/cronosd >>"$work/log" 2>&1 &
	srv=$!
	local i
	for i in $(seq 1 80); do curl -sf "$API/v1/health" >/dev/null 2>&1 && return 0; sleep 0.25; done
	return 1
}
stop() { kill "${srv:-}" 2>/dev/null || true; wait "${srv:-}" 2>/dev/null || true; srv=""; sleep 1; }
code() { curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$@"; }

start || { tail -5 "$work/log"; die "no server"; }
TOK=$(./bin/cronos-token -audience portal -role editor -org acme -project finance -subject sam)
ADMIN=$(./bin/cronos-token -audience portal -role admin -org acme -project finance -subject ada)

# -- 1. The door ---------------------------------------------------------------

say "Publishing a misspelled timezone"
sed -e 's|timezone: Europe/Berlin|timezone: Europe/Berln|' \
	-e 's|name: monthly-statements|name: typo-schedule|' \
	demo/definitions/monthly-statements.yaml >"$work/typo.yaml"

got=$(code -X POST -H "Authorization: Bearer $TOK" -H 'content-type: application/yaml' \
	--data-binary @"$work/typo.yaml" "$API/v1/definitions")
[ "$got" = 422 ] ||
	die "a schedule with timezone Europe/Berln published with $got — the next restart would not come back"
ok "is refused ($got), where the person who typed it can still see it"

said=$(curl -s -X POST -H "Authorization: Bearer $TOK" -H 'content-type: application/yaml' \
	--data-binary @"$work/typo.yaml" "$API/v1/definitions")
case "$said" in
*Europe/Berln*timezone* | *timezone*Europe/Berln*) ok "and told what was wrong with it" ;;
*) die "refused without naming the timezone: $(printf %s "$said" | head -c 120)" ;;
esac

# The same hole, in the other field a schedule needs to arm. definition.Validate
# counts five fields, because the core is standard library only; the parser that
# has to arm it is stricter, and every expression they disagreed about published
# with a 200 and never fired. "0 25 1 * *" is five fields and hour twenty-five.
for expr in "0 25 1 * *" "0 6 32 * *" "0 6 1 13 *" "* * * * 8" "0 6 1 * MOO"; do
	sed -e "s|cron: .*|cron: \"$expr\"|" -e 's|name: monthly-statements|name: cron-typo|' \
		demo/definitions/monthly-statements.yaml >"$work/cron.yaml"
	got=$(code -X POST -H "Authorization: Bearer $TOK" -H 'content-type: application/yaml' \
		--data-binary @"$work/cron.yaml" "$API/v1/definitions")
	[ "$got" = 422 ] ||
		die "a schedule with cron \"$expr\" published with $got, and it will never fire"
done
ok "and so is every cron expression the scheduler cannot parse"

# A real one still publishes, so the check is a check and not a wall.
sed -e 's|timezone: Europe/Berlin|timezone: Asia/Jakarta|' \
	-e 's|name: monthly-statements|name: jakarta-schedule|' \
	demo/definitions/monthly-statements.yaml >"$work/ok.yaml"
[ "$(code -X POST -H "Authorization: Bearer $TOK" -H 'content-type: application/yaml' \
	--data-binary @"$work/ok.yaml" "$API/v1/definitions")" = 200 ] ||
	die "a schedule in Asia/Jakarta was refused too"
ok "and a real timezone still publishes"

# -- 2. The net ----------------------------------------------------------------
#
# What is already stored: published by a build before the check existed, or
# restored from an older backup. The deployment has to come back from it, or
# the only repair is a prompt on the database.

say "A store that already holds one"
stop
# One of them, not all: the story is one editor's typo sitting among
# definitions that are fine, and the point is that the fine ones still serve.
python3 - "$work/c.db" <<'BREAK'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
db.text_factory = bytes
patched = 0
rows = db.execute(
    "SELECT rowid, body FROM cronos_definitions WHERE kind='Schedule' AND name='jakarta-schedule'"
).fetchall()
for rid, body in rows:
    broken = body.replace(b"Asia/Jakarta", b"Europe/Berln")
    if broken != body:
        db.execute("UPDATE cronos_definitions SET body=? WHERE rowid=?", (broken, rid))
        patched += 1
db.commit()
raise SystemExit(0 if patched else "no schedule to break")
BREAK
ok "broke one stored schedule's timezone, the way an older build would have left it"

start || die "the deployment did not come back — one stored definition took every project off the air"
ok "it starts"

[ "$(code -H "Authorization: Bearer $ADMIN" "$API/v1/catalog")" = 200 ] || die "the catalogue does not answer"
figure=$(curl -s --max-time 15 -X POST -H "Authorization: Bearer $ADMIN" \
	-H 'content-type: application/json' -d '{}' "$API/v1/reports/billing-summary" |
	python3 -c 'import json,sys
d = json.load(sys.stdin)
print(d["blocks"][0]["value"] if "blocks" in d else "refused")')
[ "$figure" != refused ] || die "reports do not render, and none of them mention a timezone"
ok "and reports still render, reading $figure — a schedule is not a report"

# Loud, which is the whole risk of not refusing. A definition that silently
# disappeared is the failure this is not allowed to become.
grep -q "definition refused" "$work/log" ||
	die "nothing in the log says a definition was dropped"
ok "the log says which definitions it would not serve"

refused=$(curl -s "$API/v1/metrics" | awk '/^cronos_definitions_refused /{print $2}')
[ "$refused" = 1 ] ||
	die "cronos_definitions_refused is $refused, and exactly one definition was broken"
ok "and cronos_definitions_refused is $refused, which is alertable"

# The rest are still being served, which is the whole trade this makes.
kept=$(curl -s -H "Authorization: Bearer $ADMIN" "$API/v1/definitions" |
	python3 -c 'import json,sys; print(len(json.load(sys.stdin)["definitions"]))')
[ "$kept" -ge 9 ] || die "only $kept definitions survived one bad schedule"
ok "and the other $kept definitions are being served"

# -- 3. And it can be repaired -------------------------------------------------

say "Repairing it, through the API"
[ "$(code -X DELETE -H "Authorization: Bearer $ADMIN" \
	"$API/v1/definitions/Schedule/jakarta-schedule")" = 204 ] ||
	die "the bad definition cannot be deleted"
[ "$(code -X POST -H "Authorization: Bearer $ADMIN" -H 'content-type: application/yaml' \
	--data-binary @"$work/ok.yaml" "$API/v1/definitions")" = 200 ] ||
	die "the corrected definition cannot be published"
ok "the bad one is deleted and a corrected one published — no database prompt"

stop
start || die "it does not come back after the repair"
[ "$(curl -s "$API/v1/metrics" | awk '/^cronos_definitions_refused /{print $2}')" = 0 ] ||
	die "something is still refused after the repair"
ok "and the next restart refuses nothing"

printf '\n  \033[32ma typo is refused, and one already stored is survivable\033[0m\n\n'
