#!/usr/bin/env bash
#
# The boundaries, asserted against a running server rather than reasoned about.
#
# cronos is embedded in other people's applications, so the questions here are
# the ones that decide whether it can be sold: can an end customer read another
# customer's rows, can one organisation see another's, can a token be edited
# into a better one. Each has an answer in the code and none of them had an
# answer anybody had checked from outside.
#
# This came out of a security review. A review is a day somebody spends and a
# document nobody reads; the same probes as a check run on every push, and the
# ones that matter are cheap:
#
#   1. an embed token reaches its own report and nothing else
#   2. row scope holds, and cannot be widened from the request
#   3. a token cannot be edited into a better one
#   4. one organisation cannot see another's anything
#   5. a viewer can read and cannot change
#   6. a share link cannot be guessed, and stops working when revoked
#
#   ./scripts/live-boundaries.sh
#
# Needs go. Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

PORT=8785; API="http://localhost:$PORT"
work=$(mktemp -d)
cleanup() {
	rm -rf "$work" || true
	[ -n "${srv:-}" ] && { kill -9 "$srv" 2>/dev/null || true; }
	return 0
}
trap cleanup EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; [ -f "$work/log" ] && tail -15 "$work/log" >&2; exit 1; }
say(){ printf '\n\033[1m%s\033[0m\n' "$*"; }

curl -s -o /dev/null --max-time 1 "$API/" 2>/dev/null && die "something is already listening on $PORT"

export CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef
go build -o bin/cronosd ./cmd/cronosd
go build -o bin/cronos-token ./cmd/cronos-token

mkdir -p "$work/defs" "$work/out"
cp demo/definitions/*.yaml "$work/defs/"

env CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
	CRONOS_SEED=demo/seed.sql CRONOS_ADMIN_KEY=an-admin-key \
	CRONOS_ORG=acme CRONOS_PROJECT=finance CRONOS_DELIVERIES="$work/out" \
	./bin/cronosd >"$work/log" 2>&1 & srv=$!
for _ in $(seq 1 60); do curl -sf "$API/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$API/v1/health" >/dev/null || die "no server"

mint() { ./bin/cronos-token "$@"; }
C1=$(mint -scope customer_id=c-1 -report billing-summary -org acme -project finance)
C2=$(mint -scope customer_id=c-2 -report billing-summary -org acme -project finance)
ADA=$(mint -audience portal -role admin -org acme -project finance -subject ada)
SAM=$(mint -audience portal -role viewer -org acme -project finance -subject sam)
EVE=$(mint -audience portal -role admin -org rival -project finance -subject eve)

code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
post() { code -X POST -H 'content-type: application/json' -d '{}' "$@"; }
total() {
	curl -s -X POST -H "Authorization: Bearer $1" -H 'content-type: application/json' \
		-d "${2:-\{\}}" "$API$3" |
		python3 -c 'import json,sys
d = json.load(sys.stdin)
print(d["blocks"][0]["value"] if "blocks" in d else "refused")'
}

# -- 1. What an end customer's token reaches ----------------------------------

say "An embed token"
[ "$(post -H "Authorization: Bearer $C1" "$API/v1/embed/reports/billing-summary")" = 200 ] ||
	die "an embed token cannot read the report it was issued for"
ok "reads the report it was issued for"

[ "$(post -H "Authorization: Bearer $C1" "$API/v1/embed/reports/customer-overview")" = 403 ] ||
	die "an embed token read a report it was not issued for"
ok "and no other report"

for path in /v1/catalog /v1/definitions /v1/runs /v1/people /v1/platform/tenants; do
	got=$(code -H "Authorization: Bearer $C1" "$API$path")
	[ "$got" = 401 ] || die "an embed token reached $path ($got)"
done
ok "and nothing an author would use"

[ "$(post -H "Authorization: Bearer $C1" "$API/v1/reports/billing-summary")" = 401 ] ||
	die "an embed token reached the portal's own read path"
ok "and not the portal's read path, which is a different audience"

# -- 2. Row scope -------------------------------------------------------------

say "Row scope"
one=$(total "$C1" '{}' /v1/embed/reports/billing-summary)
two=$(total "$C2" '{}' /v1/embed/reports/billing-summary)
all=$(total "$ADA" '{}' /v1/reports/billing-summary)
[ "$one" != "$two" ] || die "two customers see the same figure ($one) — row scope is not applied"
[ "$one" != "$all" ] || die "a customer sees the whole project ($all)"
ok "two customers see their own figures ($one and $two, of $all)"

widened=$(total "$C1" '{"filters":{"customer_id":{"op":"in","values":["c-1","c-2"]}}}' \
	/v1/embed/reports/billing-summary)
[ "$widened" = "$one" ] ||
	die "a customer widened their own scope with a filter: $widened rather than $one"
ok "and a filter naming another customer does not widen it"

# A parameter and a scope are both refused rather than quietly honoured.
for body in '{"params":{"customer_id":"c-2"}}' '{"scope":{"customer_id":"c-2"}}'; do
	got=$(total "$C1" "$body" /v1/embed/reports/billing-summary)
	[ "$got" = refused ] || die "a customer set their own scope with $body and got $got"
done
ok "and neither a parameter nor a scope in the body is honoured"

# -- 3. Forging ---------------------------------------------------------------

say "A token somebody edited"
forged=$(python3 -c "
import base64, json
tok = '$C1'
body, sig = tok[3:].split('.')
pad = lambda s: s + '=' * (-len(s) % 4)
c = json.loads(base64.urlsafe_b64decode(pad(body)))
c['scp'] = {'customer_id': 'c-2'}   # somebody else's rows
c['plt'] = True                     # and the platform, while we are here
print('v1.' + base64.urlsafe_b64encode(json.dumps(c).encode()).decode().rstrip('=') + '.' + sig)")
[ "$(post -H "Authorization: Bearer $forged" "$API/v1/embed/reports/billing-summary")" = 401 ] ||
	die "claims were rewritten and the signature still passed"
ok "rewritten claims with the original signature are refused"

elsewhere=$(CRONOS_SIGNING_KEY=ffffffffffffffffffffffffffffffff \
	mint -scope customer_id=c-2 -report billing-summary -org acme -project finance)
[ "$(post -H "Authorization: Bearer $elsewhere" "$API/v1/embed/reports/billing-summary")" = 401 ] ||
	die "a token signed with another key was accepted"
ok "and a token signed with another key is too"

# -- 4. Another organisation --------------------------------------------------

say "Another organisation's administrator"
[ "$(code -H "Authorization: Bearer $EVE" "$API/v1/catalog")" = 403 ] ||
	die "another organisation read the catalogue"
ok "cannot read the catalogue"

for path in /v1/definitions /v1/runs /v1/people /v1/shares; do
	body=$(curl -s -H "Authorization: Bearer $EVE" "$API$path")
	case "$body" in
	*'[]'*) ;;
	*) die "$path answered another organisation with something: $(printf %s "$body" | head -c 80)" ;;
	esac
done
ok "and every list it can reach is empty rather than somebody else's"

[ "$(code -X POST -H "Authorization: Bearer $EVE" -H 'content-type: application/yaml' \
	--data-binary @demo/definitions/invoices.yaml "$API/v1/definitions")" = 403 ] ||
	die "another organisation published into this project"
ok "and it cannot publish here"

# -- 5. A viewer --------------------------------------------------------------

say "A viewer in this project"
[ "$(post -H "Authorization: Bearer $SAM" "$API/v1/reports/billing-summary")" = 200 ] ||
	die "a viewer cannot read a report"
ok "reads a report"

[ "$(code -X POST -H "Authorization: Bearer $SAM" -H 'content-type: application/yaml' \
	--data-binary @demo/definitions/invoices.yaml "$API/v1/definitions")" = 403 ] ||
	die "a viewer published"
[ "$(code -X DELETE -H "Authorization: Bearer $SAM" "$API/v1/definitions/Report/billing-summary")" = 403 ] ||
	die "a viewer deleted a definition"
[ "$(code -X POST -H "Authorization: Bearer $SAM" "$API/v1/runs/run_anything/resume")" = 403 ] ||
	die "a viewer resumed a run"
[ "$(code -H "Authorization: Bearer $SAM" "$API/v1/people")" = 403 ] ||
	die "a viewer read the roster"
ok "and cannot publish, delete, resume or read the roster"

# -- 6. Share links -----------------------------------------------------------

say "A share link"
# A row-scoped report cannot be shared at all, which is its own protection: the
# link has no customer, so it would either show everything or nothing.
refused=$(curl -s -X POST -H "Authorization: Bearer $ADA" -H 'content-type: application/json' \
	-d '{"report":"billing-summary","expiresIn":3600}' "$API/v1/shares")
case "$refused" in
*"scoped per customer"*) ok "a row-scoped report cannot be shared publicly" ;;
*) die "a row-scoped report was shared: $(printf %s "$refused" | head -c 90)" ;;
esac

id=$(curl -s -X POST -H "Authorization: Bearer $ADA" -H 'content-type: application/json' \
	-d '{"report":"customer-overview","expiresIn":3600}' "$API/v1/shares" |
	python3 -c 'import json,sys
d = json.load(sys.stdin)
print(d.get("id") or d.get("url", "").rsplit("/", 1)[-1])')
[ -n "$id" ] || die "could not create a share"
[ "${#id}" -ge 20 ] || die "a share id is only ${#id} characters — that is guessable"
ok "has an id of ${#id} characters"

[ "$(code -X POST "$API/v1/shares/$id/open")" = 200 ] || die "a share does not open"
[ "$(code -X POST "$API/v1/shares/shr_aaaaaaaaaaaaaaaa/open")" = 404 ] ||
	die "an id nobody issued opened something"
ok "opens with no session, and an id nobody issued does not"

curl -s -o /dev/null -X DELETE -H "Authorization: Bearer $ADA" "$API/v1/shares/$id"
[ "$(code -X POST "$API/v1/shares/$id/open")" = 404 ] || die "a revoked share still opens"
ok "and it stops opening when revoked"

# Guessing is throttled, which is the other half of an unguessable id: the
# entropy makes one guess hopeless and the limit makes a million of them slow.
#
# Last, because it trips the limiter: anything checked after this comes back
# 429 rather than what it means, which is how the revoke check above briefly
# reported that a revoked share still opened.
throttled=0
for i in $(seq 1 60); do
	[ "$(code -X POST "$API/v1/shares/shr_$(printf '%016x' "$i")/open")" = 429 ] &&
		throttled=$((throttled + 1))
done
[ "$throttled" -gt 20 ] || die "only $throttled of 60 guesses were throttled"
ok "and guessing is throttled ($throttled of 60 refused)"

printf '\n  \033[32mthe boundaries hold\033[0m\n\n'
