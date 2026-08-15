#!/usr/bin/env bash
#
# A rolling deploy, with two real binaries built from two real commits.
#
# Every other check runs one version. A deploy runs two: for the length of a
# rollout the old instances are still serving the database the new one has just
# migrated, and whether that works is decided by how the migrations were
# written, months earlier, by somebody who was not thinking about it.
#
# Three questions, and the third is the one nobody asks until it is six in the
# morning:
#
#   1. old serving, new migrates → does the old one keep working?
#   2. and can it still write, or only read?
#   3. old restarts after the migration → does it come back?
#
# The answers are yes, yes, and no — deliberately. An old build refuses to open
# a database a newer one has migrated, and says so in a sentence rather than
# failing somewhere further in. That is the right refusal and it is also the
# whole operational story of an upgrade: the migration is the point of no
# return, and it happens on the first new instance to start. See "Upgrading" in
# docs/deploying.md.
#
#   ./scripts/live-upgrade.sh
#
# Needs go, podman (it starts a Postgres) and a git checkout with history: the
# old build comes from a worktree at the commit before the newest migration.
# Leaves nothing behind, including the worktree.
set -euo pipefail
cd "$(dirname "$0")/.."

command -v podman >/dev/null 2>&1 || { echo "skipped: needs podman for a database"; exit 0; }

OLD_PORT=8801; NEW_PORT=8802
OLD="http://localhost:$OLD_PORT"; NEW="http://localhost:$NEW_PORT"
PG=cronos-upgrade-test; PGPORT=55435
work=$(mktemp -d)
tree=$work/old-checkout

cleanup() {
	[ -n "${oldsrv:-}" ] && { kill -9 "$oldsrv" 2>/dev/null || true; }
	[ -n "${newsrv:-}" ] && { kill -9 "$newsrv" 2>/dev/null || true; }
	podman rm -f "$PG" >/dev/null 2>&1 || true
	git worktree remove --force "$tree" >/dev/null 2>&1 || true
	rm -rf "$work" || true
	return 0
}
trap cleanup EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }
say(){ printf '\n\033[1m%s\033[0m\n' "$*"; }

for p in "$OLD_PORT" "$NEW_PORT"; do
	curl -s -o /dev/null --max-time 1 "http://localhost:$p/" 2>/dev/null &&
		die "something is already listening on $p"
done

export CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef

# -- Two builds ---------------------------------------------------------------
#
# "Old" is the commit before the newest migration landed, found from the
# history rather than pinned: a pinned hash is a number that is right once. If
# the newest migration has no commit of its own — a fresh clone with no
# history, a squash — there is nothing to compare against and this check has
# nothing to say.

say "Two versions"
go build -o "$work/cronosd-new" ./cmd/cronosd || die "build"

newest=$(grep -o 'CREATE TABLE IF NOT EXISTS cronos_[a-z_]*' \
	internal/adapter/store/sql/migrate.go | tail -2 | head -1 | awk '{print $NF}')
[ -n "$newest" ] || die "could not find the newest migration's table"

added=$(git log --format=%H -S "$newest" -- internal/adapter/store/sql/migrate.go | tail -1)
[ -n "$added" ] || { echo "  skipped: no history for $newest"; exit 0; }
before=$(git rev-parse --verify "$added^" 2>/dev/null) ||
	{ echo "  skipped: $newest was added in the first commit"; exit 0; }

git worktree add -q --detach "$tree" "$before" || die "could not check out $before"
(cd "$tree" && go build -o "$work/cronosd-old" ./cmd/cronosd) ||
	die "the old commit does not build — pick a different one"
printf '  new: HEAD\n  old: %s, the commit before %s\n' "${before:0:12}" "$newest"

# -- A database ---------------------------------------------------------------

podman rm -f "$PG" >/dev/null 2>&1 || true
podman run -d --name "$PG" -e POSTGRES_PASSWORD=cronos -e POSTGRES_USER=cronos \
	-e POSTGRES_DB=cronos -p "$PGPORT:5432" docker.io/library/postgres:16-alpine >/dev/null
for _ in $(seq 1 90); do podman exec "$PG" pg_isready -U cronos >/dev/null 2>&1 && break; sleep 1; done
podman exec "$PG" pg_isready -U cronos >/dev/null 2>&1 || die "the database never came up"

DSN="postgres://cronos:cronos@localhost:$PGPORT/cronos?sslmode=disable"
mkdir -p "$work/defs"
cp demo/definitions/*.yaml "$work/defs/"

# The demo warehouse is an in-memory SQLite, which is per process: the two
# builds would each hold their own copy and disagree about every figure for a
# reason that has nothing to do with either of them. One file, seeded once.
sed -i.bak "s|dsn: .*|dsn: \"file:$work/warehouse.db\"|" "$work/defs/warehouse.yaml"
rm -f "$work/defs/warehouse.yaml.bak"
grep -q "file:$work/warehouse.db" "$work/defs/warehouse.yaml" ||
	die "could not point the demo warehouse at a file both builds can read"

start() { # start <binary> <port> <logname> [extra env...]
	local bin=$1 port=$2 name=$3; shift 3
	env CRONOS_ADDR=":$port" CRONOS_DEFINITIONS="$work/defs" \
		CRONOS_STORE_DRIVER=postgres CRONOS_STORE_DSN="$DSN" \
		CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
		CRONOS_ORG=acme CRONOS_PROJECT=finance "$@" \
		"$bin" >"$work/$name.log" 2>&1 &
	echo $!
}
up() { # up <url>
	local i
	for i in $(seq 1 80); do curl -sf "$1/v1/health" >/dev/null 2>&1 && return 0; sleep 0.25; done
	return 1
}
code() { curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$@"; }
figure() {
	curl -s --max-time 20 -X POST -H "Authorization: Bearer $TOK" \
		-H 'content-type: application/json' -d '{}' "$1/v1/reports/billing-summary" |
		python3 -c 'import json,sys
d = json.load(sys.stdin)
print(d["blocks"][0]["value"] if "blocks" in d else "refused")' 2>/dev/null || echo unreachable
}

# -- 1. The old version, on its own schema ------------------------------------

say "Before the deploy"
oldsrv=$(start "$work/cronosd-old" "$OLD_PORT" old CRONOS_SEED=demo/seed.sql)
up "$OLD" || { tail -5 "$work/old.log"; die "the old build never came up"; }

curl -s -o /dev/null -X POST -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","name":"Ada","password":"a-password-for-ada","org":"acme","project":"finance"}' \
	"$OLD/v1/setup"
TOK=$(curl -s "$OLD/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","password":"a-password-for-ada"}' |
	python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))')
[ -n "$TOK" ] || die "could not sign in to the old build"

was=$(figure "$OLD")
[ "$was" != refused ] && [ "$was" != unreachable ] || die "the old build does not render"
ok "the old build is serving, with an account and a report reading $was"

schema_before=$(podman exec "$PG" psql -U cronos -d cronos -tAc \
	"SELECT count(*) FROM cronos_schema_migrations")

# -- 2. The new one starts beside it and migrates -----------------------------

say "During the deploy"
newsrv=$(start "$work/cronosd-new" "$NEW_PORT" new)
up "$NEW" || { tail -5 "$work/new.log"; die "the new build never came up beside the old one"; }

schema_after=$(podman exec "$PG" psql -U cronos -d cronos -tAc \
	"SELECT count(*) FROM cronos_schema_migrations")
[ "$schema_after" -gt "$schema_before" ] ||
	die "the new build migrated nothing ($schema_before then $schema_after) — the two versions share a schema and this check proves nothing"
ok "the new build started and migrated $schema_before → $schema_after"

# The property the whole design of the migrations exists to give: they only add
# tables, so code that has never heard of the new ones is unaffected. It stops
# being true the first time somebody writes ALTER TABLE.
[ "$(code "$OLD/v1/health")" = 200 ] || die "the old build's liveness failed after the migration:
$(tail -5 "$work/old.log")"
for p in /v1/catalog /v1/runs /v1/people /v1/definitions /v1/shares; do
	got=$(code -H "Authorization: Bearer $TOK" "$OLD$p")
	[ "$got" = 200 ] || die "the old build's $p answered $got after the migration"
done
now=$(figure "$OLD")
[ "$now" = "$was" ] || die "the old build reads $now after the migration, and $was before it"
ok "the old build still answers every route, and still reads $was"

# Reading is the easy half. A rollout that only lets the old instances read is
# a rollout with a write outage in the middle of it.
[ "$(code -X POST -H "Authorization: Bearer $TOK" -H 'content-type: application/yaml' \
	--data-binary @demo/definitions/invoices.yaml "$OLD/v1/definitions")" = 200 ] ||
	die "the old build cannot publish after the migration — a rollout would be a write outage"
[ "$(code -X POST "$OLD/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","password":"a-password-for-ada"}')" = 200 ] ||
	die "nobody can sign in to the old build after the migration"
ok "and can still publish, and still sign people in"

# And the new one is serving the same database at the same time.
[ "$(figure "$NEW")" = "$was" ] || die "the two versions disagree about the figure"
ok "both versions serve the same database and agree on the number"

# The old build's own readiness is not this repo's behaviour to assert — it is
# whatever that commit shipped, and a build already deployed cannot be fixed.
# Reported rather than checked, because it is the difference an operator sees
# between upgrading from a version before this one and after it.
printf '  \033[2m--\033[0m the old build reports readiness %s during the rollout\n' \
	"$(code "$OLD/v1/ready")"

# -- 2b. And this build, when a newer one migrates past it -------------------
#
# The same position the old build is in, for the build being released. There is
# no newer cronos to run, so the schema is moved on the way one would leave it.
# This is the defect the two-binary run above found: readiness is what keeps an
# instance in the load balancer, and reporting down for a schema that is merely
# ahead took every old instance out of rotation the moment the first new one
# migrated — a deployment serving all of its traffic from one pod, mid-deploy.

ahead=$(( schema_after + 1 ))
podman exec "$PG" psql -U cronos -d cronos -v ON_ERROR_STOP=1 -tAc \
	"INSERT INTO cronos_schema_migrations (id, name, applied_at)
	 VALUES ($ahead, 'from-a-newer-cronos', '2026-01-01T00:00:00Z')" >/dev/null ||
	die "could not move the schema on"

# Readiness caches for five seconds, so the answer has to be waited for rather
# than read once.
still=0
for _ in $(seq 1 20); do
	[ "$(code "$NEW/v1/ready")" = 200 ] && still=$((still + 1))
	sleep 0.5
done
[ "$still" -ge 18 ] ||
	die "this build reports not-ready when a newer one has migrated ($still of 20 probes ready) — every instance would leave the load balancer at once"
[ "$(figure "$NEW")" = "$was" ] || die "this build stopped serving at a schema one version ahead"
ok "this build stays ready, and serving, at a schema a newer one migrated"

podman exec "$PG" psql -U cronos -d cronos -tAc \
	"DELETE FROM cronos_schema_migrations WHERE id = $ahead" >/dev/null

# -- 3. And the old one cannot come back --------------------------------------
#
# The half that decides the runbook. An old instance that is restarted — a
# node eviction, an OOM, a rollback — does not come back, by design: it refuses
# rather than opening a schema it does not understand and writing rows that are
# missing whatever the newer one added.

say "After the deploy"
kill "$oldsrv" 2>/dev/null || true
wait "$oldsrv" 2>/dev/null || true
oldsrv=""

again=$(start "$work/cronosd-old" "$OLD_PORT" old-restart)
sleep 3
if curl -sf "$OLD/v1/health" >/dev/null 2>&1; then
	kill -9 "$again" 2>/dev/null || true
	die "the old build opened a schema a newer one had migrated — it would write rows the new schema expects more of"
fi
kill -9 "$again" 2>/dev/null || true

grep -q "migrated by a newer cronos" "$work/old-restart.log" ||
	die "the old build refused, but not with a sentence that says why:
$(tail -3 "$work/old-restart.log")"
ok "a restarted old build refuses, and names the reason"

printf '\n  \033[32ma rollout is safe and one-way\033[0m — old instances keep serving; a restarted one does not come back\n\n'
