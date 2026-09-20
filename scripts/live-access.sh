#!/usr/bin/env bash
#
# Assigning people to a report, and everybody else being kept out of it.
#
# A project role answers "may this person read reports here", which is one
# question short: a finance department has people who should read the
# receivables summary and people who should not. Grants answer the second, and
# the property that matters is not that the granted person can open the report
# — it is that the five other ways into a report all refuse the people who
# were not named.
#
# That is why this is a live check rather than a unit test. The decision itself
# is one function with no database in it and is covered where it lives; what
# has been wrong here before is the wiring. A grant check has twice been added
# to a handler as a field and a builder with the call itself never inserted:
# it compiled, every test passed, and the endpoint went on serving a restricted
# report. Only a real request finds that.
#
#   ./scripts/live-access.sh
#
# Needs go. Leaves nothing behind.
set -euo pipefail

cd "$(dirname "$0")/.."

PORT=8797
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

token() { python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))'; }

# Somebody in this project, by email, name and role. Returns their account id,
# which is what a grant to a person names.
person() {
	curl -s -X POST -H "Authorization: Bearer $ADMIN" -H 'content-type: application/json' \
		-d "{\"email\":\"$1\",\"name\":\"$2\",\"role\":\"$3\",\"password\":\"another-password-here\"}" \
		"$API/v1/people" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("id",""))'
}
signin() {
	curl -s -X POST -H 'content-type: application/json' \
		-d "{\"email\":\"$1\",\"password\":\"another-password-here\"}" "$API/v1/auth/login" | token
}
catalogue() {
	curl -s -H "Authorization: Bearer $1" "$API/v1/catalog" |
		python3 -c 'import json,sys; print(" ".join(sorted(r["name"] for r in json.load(sys.stdin).get("reports",[]))))'
}
# The HTTP code of one request as one person.
as() {
	local who=$1 method=$2 path=$3 body=${4:-}
	if [ -n "$body" ]; then
		curl -s -o /dev/null -w '%{http_code}' -X "$method" -H "Authorization: Bearer $who" \
			-H 'content-type: application/json' -d "$body" "$API$path"
	else
		curl -s -o /dev/null -w '%{http_code}' -X "$method" -H "Authorization: Bearer $who" "$API$path"
	fi
}
grant() {
	curl -s -o /dev/null -w '%{http_code}' -X "${4:-POST}" \
		-H "Authorization: Bearer $ADMIN" -H 'content-type: application/json' \
		-d "{\"kind\":\"$2\",\"subject\":\"$3\"}" "$API/v1/reports/$1/grants"
}

# --- a project with two reports and four people -------------------------------

say "A project, and the people in it"
go build -o bin/cronosd ./cmd/cronosd || die "build"
mkdir -p "$work/defs"
cp demo/definitions/*.yaml "$work/defs/"

CRONOS_ADDR=":$PORT" \
	CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=sqlite \
	CRONOS_STORE_DSN="file:$work/cronos.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_SEED=demo/seed.sql CRONOS_SEED_SOURCE=warehouse \
	./bin/cronosd >"$work/log" 2>&1 &
server=$!
for _ in $(seq 1 60); do
	curl -sf "$API/v1/health" >/dev/null 2>&1 && break
	sleep 0.25
done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/log"; die "cronos never came up"; }

ADMIN=$(curl -s -X POST -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","name":"Ada Rahayu","password":"a-password-they-chose","org":"acme","project":"finance"}' \
	"$API/v1/setup" | token)
[ -n "$ADMIN" ] || { cat "$work/log"; die "no session came back from /v1/setup"; }

BUDI=$(person budi@acme.example "Budi" viewer)
CITRA=$(person citra@acme.example "Citra" viewer)
DEWI=$(person dewi@acme.example "Dewi" editor)
[ -n "$BUDI" ] && [ -n "$CITRA" ] && [ -n "$DEWI" ] || die "the roster could not be created"

budi=$(signin budi@acme.example)
citra=$(signin citra@acme.example)
dewi=$(signin dewi@acme.example)
[ -n "$budi" ] && [ -n "$citra" ] && [ -n "$dewi" ] || die "somebody could not sign in"
ok "an administrator, two viewers and an editor"

# The state before anybody is assigned. A report nobody granted is open to the
# project, which is what makes turning this on a no-op rather than an outage.
case "$(catalogue "$budi")" in
*customer-overview*) ok "and a report nobody has restricted is open to all of them" ;;
*) die "a report with no grants was already hidden" ;;
esac

# --- assigning one person -----------------------------------------------------

say "Assigning one person to one report"
[ "$(grant customer-overview user "$BUDI")" = 200 ] || die "the grant was refused"

case "$(catalogue "$budi")" in
*customer-overview*) ok "the person named in it still sees the report" ;;
*) die "the person the report was granted to cannot see it" ;;
esac
case "$(catalogue "$citra")" in
*customer-overview*) die "a viewer nobody named still sees a restricted report" ;;
*) ok "a viewer nobody named no longer sees it at all" ;;
esac
# An editor can rewrite the report's YAML and cannot grant themselves access to
# it, which is the whole reason grants are recorded against the report rather
# than inside it.
case "$(catalogue "$dewi")" in
*customer-overview*) die "an editor nobody named still sees a restricted report" ;;
*) ok "and neither does an editor, who can still edit its definition" ;;
esac
case "$(catalogue "$ADMIN")" in
*customer-overview*) ok "the administrator is not locked out of what they administer" ;;
*) die "the administrator lost the report they just granted" ;;
esac

# --- every way into a report --------------------------------------------------

say "Every path that opens a report, as somebody who was not assigned"
for check in \
	"POST /v1/reports/customer-overview|running it" \
	"GET /v1/definitions/Report/customer-overview|reading its definition" \
	"POST /v1/reports/customer-overview/send|mailing it to themselves"; do

	request=${check%%|*}
	what=${check##*|}
	got=$(as "$citra" "${request%% *}" "${request##* }" '{}')
	case "$got" in
	403 | 404) ok "$what is refused ($got)" ;;
	*) die "$what answered $got — a restricted report is open through that path" ;;
	esac
done

# A share is the one route in this API with no credential behind it, so minting
# one for a report the caller may not open converts a refusal into anonymous
# access that outlives the refusal.
got=$(as "$citra" POST /v1/shares \
	'{"report":"customer-overview","output":"interactive","expiresIn":"1h"}')
case "$got" in
403 | 404) ok "minting a public link for it is refused ($got)" ;;
*) die "a public link to a restricted report was minted: $got" ;;
esac

# And the one that renders it for everybody at once.
got=$(as "$citra" POST /v1/schedules/monthly-statements/run '{}')
case "$got" in
403 | 404) ok "running its schedule by hand is refused ($got)" ;;
*) die "a schedule over a restricted report ran for somebody not named: $got" ;;
esac

# The granted person, on the same path, to prove the refusals above are the
# grant and not a report that is broken for everybody.
[ "$(as "$budi" POST /v1/reports/customer-overview '{}')" = 200 ] ||
	die "the person who was assigned cannot open the report"
ok "and the person who was assigned opens it (200)"

# --- assigning a group --------------------------------------------------------

say "Assigning a group"
gid=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" -H 'content-type: application/json' \
	-d '{"name":"receivables"}' "$API/v1/groups" |
	python3 -c 'import json,sys; print(json.load(sys.stdin).get("id",""))')
[ -n "$gid" ] || die "the group could not be created"

[ "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $ADMIN" \
	-H 'content-type: application/json' -d "{\"user\":\"$CITRA\"}" \
	"$API/v1/groups/$gid/members")" = 200 ] || die "Citra could not be added to the group"
# By name rather than by id: a grant names a group the way a person reads it,
# and names are unique per project.
[ "$(grant customer-overview group receivables)" = 200 ] || die "the group grant was refused"

case "$(catalogue "$citra")" in
*customer-overview*) ok "a member of the granted group sees it" ;;
*) die "a member of the granted group still cannot see the report" ;;
esac
[ "$(as "$citra" POST /v1/reports/customer-overview '{}')" = 200 ] ||
	die "a member of the granted group cannot open the report"
ok "and opens it"
case "$(catalogue "$dewi")" in
*customer-overview*) die "granting a group opened the report to somebody outside it" ;;
*) ok "and the editor, who is in no group, still does not" ;;
esac

# --- taking it back -----------------------------------------------------------

say "Taking access back"
[ "$(grant customer-overview user "$BUDI" DELETE)" = 200 ] || die "the grant could not be revoked"
case "$(catalogue "$budi")" in
*customer-overview*) die "a revoked grant still opens the report" ;;
*) ok "revoking closes it on the next request" ;;
esac

[ "$(grant customer-overview group receivables DELETE)" = 200 ] || die "the group grant could not be revoked"
# The last grant going means nobody restricted it any more, which is the state
# the deployment started in — and the difference between "open to everybody"
# and "granted to nobody" is the one this feature has to get right.
case "$(catalogue "$dewi")" in
*customer-overview*) ok "and the last grant going makes it open to the project again" ;;
*) die "removing every grant left the report closed to everybody" ;;
esac

restricted=$(curl -s -H "Authorization: Bearer $ADMIN" "$API/v1/reports/customer-overview/grants" |
	python3 -c 'import json,sys; print(json.load(sys.stdin)["restricted"])')
[ "$restricted" = "False" ] || die "the report still reports itself as restricted"
ok "and says so, rather than showing a padlock over an open report"

say "A report can be assigned to people, and holds against everybody else"
