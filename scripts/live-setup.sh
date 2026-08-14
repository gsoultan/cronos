#!/usr/bin/env bash
#
# The first run, and the tier above organisations.
#
# Two things that only mean anything against a real, empty database: that /setup
# is open exactly once, and that a deployment administrator can manage accounts
# across tenants without being able to read any of their data.
#
# The second is the answer this deployment was built to: administration only. A
# platform administrator who could also read every project would be one
# credential away from every customer at once. The check below is not that the
# permission exists — it is that having it opens nothing.
#
#   ./scripts/live-setup.sh
#
# Needs go. Leaves nothing behind.
set -euo pipefail

cd "$(dirname "$0")/.."

PORT=8799
API="http://localhost:$PORT"

work=$(mktemp -d)
cleanup() {
	[ -n "${srv:-}" ] && { kill "$srv" 2>/dev/null || true; }
	rm -rf "$work" || true
	return 0
}
trap 'cleanup 2>/dev/null' EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok() { printf '  \033[32mok\033[0m %s\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

post() { curl -s -X "$1" -H 'content-type: application/json' "${@:4}" -d "$3" "$API$2"; }
code() { curl -s -o /dev/null -w '%{http_code}' -X "$1" -H 'content-type: application/json' "${@:4}" -d "$3" "$API$2"; }
token() { sed 's/.*"token":"//; s/".*//'; }

say "A deployment nobody has ever signed in to"
go build -o bin/cronosd ./cmd/cronosd || die "build"
mkdir -p "$work/defs"

CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	./bin/cronosd >"$work/log" 2>&1 &
srv=$!
for _ in $(seq 1 40); do
	curl -sf "$API/v1/health" >/dev/null 2>&1 && break
	sleep 0.25
done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/log"; die "cronos never came up"; }

curl -s "$API/v1/setup" | grep -q '"needed":true' || die "a fresh deployment does not offer setup"
ok "offers to be set up"

# --- setting up ---------------------------------------------------------------

say "Setting up"

first='{"email":"ops@acme.example","name":"Ada Rahayu","password":"a-password-they-chose","org":"Acme Logistics","project":"Finance"}'
out=$(post POST /v1/setup "$first")
session=$(printf '%s' "$out" | token)
[ -n "$session" ] && [ "$session" != "$out" ] || { printf '  %s\n' "$out"; die "no session came back"; }
ok "created the first account and signed in"

# The typed names became identifiers. They are half of every tenancy check and
# part of a path when definitions live on disk, so "Acme Logistics" cannot stay
# as it was typed.
printf '%s' "$out" | grep -q '"org":"acme-logistics"' ||
	{ printf '  %s\n' "$out"; die "the organisation name was not reduced"; }
printf '%s' "$out" | grep -q '"project":"finance"' || die "the project name was not reduced"
ok "the names it was given became acme-logistics / finance"

# --- and closed ---------------------------------------------------------------

say "And closed"

curl -s "$API/v1/setup" | grep -q '"needed":false' || die "setup is still offered"
ok "no longer offered"

[ "$(code POST /v1/setup '{"email":"attacker@example.com","password":"another-password-here","org":"x","project":"y"}')" = 409 ] ||
	die "a second setup was accepted"
ok "and refuses a second account"

# --- what the permission opens ------------------------------------------------

say "What administering the deployment does, and does not, open"

curl -s -H "Authorization: Bearer $session" "$API/v1/platform/tenants" |
	grep -q 'acme-logistics' || die "the administrator cannot see the tenants"
ok "sees every tenant"

curl -s -H "Authorization: Bearer $session" "$API/v1/platform/people" |
	grep -q 'ops@acme.example' || die "the administrator cannot see the accounts"
ok "sees every account"

# Somebody else, in another organisation entirely.
[ "$(code POST /v1/people '{"email":"dewi@globex.example","name":"Dewi","role":"editor","password":"a-password-for-dewi"}' -H "Authorization: Bearer $session")" = 201 ] ||
	die "could not add somebody"
dewi=$(curl -s -H "Authorization: Bearer $session" "$API/v1/platform/people" |
	python3 -c 'import json,sys
for p in json.load(sys.stdin)["people"]:
    if p["email"] == "dewi@globex.example": print(p["id"])')
[ -n "$dewi" ] || die "the new account is not in the platform list"

[ "$(code PATCH "/v1/platform/people/$dewi" '{"org":"globex","project":"ops","role":"admin"}' -H "Authorization: Bearer $session")" = 204 ] ||
	die "could not move somebody between organisations"
ok "moves an account from one organisation to another"

# And now the line. The administrator is in acme-logistics/finance; Dewi is in
# globex/ops. Reading globex's catalogue is not something this permission opens.
theirs=$(curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"dewi@globex.example","password":"a-password-for-dewi"}' | token)
[ -n "$theirs" ] || die "the moved account cannot sign in"

# The administrator's own session names their own project, so a request carries
# that tenancy and reaches their own catalogue only. There is no route by which
# the platform permission opens another project's — asserted by there being
# none: every data route resolves the project from the caller's own principal.
mine=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $session" "$API/v1/catalog")
[ "$mine" = 200 ] || die "the administrator cannot read their own project: $mine"
ok "reads its own project, like any member"

# Proven by moving the administrator out of every project with a catalogue and
# showing the permission alone opens nothing.
me=$(curl -s -H "Authorization: Bearer $session" "$API/v1/auth/profile" |
	sed 's/.*"id":"//; s/".*//')
[ "$(code PATCH "/v1/platform/people/$me" '{"org":"acme-logistics","project":"elsewhere","role":"viewer"}' -H "Authorization: Bearer $session")" = 204 ] ||
	die "could not move the administrator"

moved=$(curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"ops@acme.example","password":"a-password-they-chose"}' | token)
[ -n "$moved" ] || die "the administrator cannot sign in after moving"

# Still a deployment administrator...
curl -s -H "Authorization: Bearer $moved" "$API/v1/platform/tenants" | grep -q globex ||
	die "moving lost the platform permission"
ok "keeps administering the deployment after moving project"

# ...and their reach into globex is exactly what their membership says, which is
# none: the catalogue they get is the one for the project their token names.
elsewhere=$(curl -s -H "Authorization: Bearer $moved" "$API/v1/catalog")
printf '%s' "$elsewhere" | grep -q 'globex' &&
	die "the platform permission opened another organisation's catalogue"
ok "and reads nothing of the project it is not in"

# --- the last administrator ---------------------------------------------------

say "The last one"

[ "$(code DELETE "/v1/platform/admins/$me" '' -H "Authorization: Bearer $moved")" = 409 ] ||
	die "the last platform administrator was revoked"
ok "cannot be revoked, because nothing could make another"

[ "$(code POST "/v1/platform/admins/$dewi" '' -H "Authorization: Bearer $moved")" = 204 ] ||
	die "could not grant it to somebody else"
ok "granting it to somebody else works"

[ "$(code DELETE "/v1/platform/admins/$me" '' -H "Authorization: Bearer $moved")" = 204 ] ||
	die "could not step down once there were two"
ok "and then stepping down works"

# Revoking ends that account's sessions, because the permission is in the token
# and would otherwise outlive the revocation by eight hours.
sleep 6
[ "$(code GET /v1/platform/tenants '' -H "Authorization: Bearer $moved")" != 200 ] ||
	die "the revoked administrator still administers"
ok "and the revoked session stops working rather than waiting to expire"

# --- the way back ---------------------------------------------------------------

say "The way back from having none"

# A deployment can still lose its last administrator: disabled, the account
# deleted, or somebody stepping down as the second-to-last while the last was
# already gone. The endpoints that grant it require the permission being
# granted, so the remedy is this command — which the documentation described
# before the command could do it.
python3 -c "
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
db.execute('DELETE FROM cronos_platform_admins')
db.commit()" "$work/c.db"

[ "$(code GET /v1/platform/tenants '' -H "Authorization: Bearer $moved")" != 200 ] ||
	die "an administrator survived the table being emptied"
ok "with none left, nobody can administer the deployment"

printf 'unused' | go run ./cmd/cronos-user \
	-dsn "file:$work/c.db" -driver sqlite -email ops@acme.example -platform \
	>"$work/grant.log" 2>&1 || { cat "$work/grant.log"; die "the CLI could not grant it"; }
case "$(cat "$work/grant.log")" in
*"deployment administrator"*) ;;
*) cat "$work/grant.log"; die "the CLI said nothing about granting it" ;;
esac
ok "cronos-user -platform grants it to an existing account"

# It takes effect on the next sign-in, because the permission travels in the
# token — the same reason revoking cuts sessions.
rescued=$(curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"ops@acme.example","password":"a-password-they-chose"}' | token)
[ -n "$rescued" ] || die "the rescued account cannot sign in"
[ "$(code GET /v1/platform/tenants '' -H "Authorization: Bearer $rescued")" = 200 ] ||
	die "the rescued account still cannot administer the deployment"
ok "and the next sign-in carries it"

# And it did not touch the password, which is somebody else's.
[ "$(code POST /v1/auth/login '{"email":"ops@acme.example","password":"unused"}')" != 200 ] ||
	die "the CLI reset the password it was given on stdin"
ok "without touching the password it never asked for"

# --- onboarding a customer ------------------------------------------------------

say "Standing up a customer that has no account at all"

# The verb this tier was missing. Before it, onboarding meant adding the person
# to your own project through the ordinary endpoint and then moving them — a
# two-step workaround for the primary job of the whole tier.
made=$(post POST /v1/platform/people \
	'{"email":"ops@globex.example","name":"Rin","org":"globex","project":"warehouse","role":"admin","password":"a-password-for-rin"}' \
	-H "Authorization: Bearer $rescued")
case "$made" in
*'"org":"globex"'*) ;;
*) printf '  %s\n' "$made"; die "the account was not created in globex" ;;
esac
case "$made" in
*'"project":"warehouse"'*) ;;
*) die "the account is in the wrong project" ;;
esac
ok "created directly in globex/warehouse"

# And it works: they can sign in, which is what "onboarded" has to mean.
theirs2=$(curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"ops@globex.example","password":"a-password-for-rin"}' | token)
[ -n "$theirs2" ] || die "the new customer cannot sign in"
ok "and they can sign in"

# The same request from an ordinary project administrator reaches nothing. This
# is the one change no project administrator may make, which is why it lives
# behind the platform check rather than beside /v1/people.
[ "$(code POST /v1/platform/people '{"email":"x@evil.example","org":"acme-logistics","project":"finance","role":"admin","password":"a-password-for-them"}' -H "Authorization: Bearer $theirs2")" = 404 ] ||
	die "an ordinary administrator created an account in another organisation"
ok "and an ordinary administrator cannot do the same"

say "All of it worked."
