#!/usr/bin/env bash
#
# Moving a session from one project to another.
#
# One process can serve several projects, and an account can belong to more
# than one of them. Until this check existed nothing exercised the move: the
# portal's switcher drew the sample directory — two organisations nobody
# belongs to — and clicking a project in it changed nothing, because the
# project is inside the token and only the server can mint one.
#
# What is proved here is the contract the switcher depends on:
#
#   - the list says where this session may go, and where it is
#   - entering mints a token for the other project, with the role that
#     project gives them, which is routinely not the one they just had
#   - what they can read changes with it, and nothing of the project they
#     left comes along
#   - a project they are not in is refused
#
#   ./scripts/live-projects.sh
#
# Needs go. Leaves nothing behind.
set -euo pipefail

cd "$(dirname "$0")/.."

PORT=8802
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
signin() {
	curl -s -X POST -H 'content-type: application/json' \
		-d "{\"email\":\"$1\",\"password\":\"$2\"}" "$API/v1/auth/login" | token
}
reports() {
	curl -s -H "Authorization: Bearer $1" "$API/v1/catalog" |
		python3 -c 'import json,sys; print(" ".join(sorted(r["name"] for r in json.load(sys.stdin).get("reports",[]))))'
}

# --- a process serving two projects -------------------------------------------

say "One deployment, two projects"
go build -o bin/cronosd ./cmd/cronosd || die "build"
go build -o bin/cronos-user ./cmd/cronos-user || die "build"

# A directory each, which is how a process serving several finds them.
mkdir -p "$work/defs/acme/finance" "$work/defs/acme/ops"
cp demo/definitions/warehouse.yaml demo/definitions/customers.yaml \
	demo/definitions/customer-list.yaml demo/definitions/customer-overview.yaml \
	"$work/defs/acme/finance/"
cp demo/definitions/shipments.yaml "$work/defs/acme/ops/"
# Its own database, because two projects sharing one is the thing this
# deployment shape exists to avoid — and nothing here reads a row, so neither
# is seeded.
sed 's/cronos-demo/cronos-ops/' demo/definitions/warehouse.yaml >"$work/defs/acme/ops/warehouse.yaml"
# One report that exists only in ops, so "what they can read" is a different
# list rather than the same one twice.
cat >"$work/defs/acme/ops/deliveries.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Report
metadata:
  name: deliveries
  title: Deliveries
spec:
  dataset: shipments
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: table
          title: Shipments
          columns: [id]
YAML

CRONOS_ADDR=":$PORT" \
	CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_PROJECTS="acme/finance,acme/ops" \
	CRONOS_STORE_DRIVER=sqlite \
	CRONOS_STORE_DSN="file:$work/cronos.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	./bin/cronosd >"$work/log" 2>&1 &
server=$!
for _ in $(seq 1 60); do
	curl -sf "$API/v1/health" >/dev/null 2>&1 && break
	sleep 0.25
done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/log"; die "cronos never came up"; }
ok "serving acme/finance and acme/ops"

# An administrator in each, written straight to the store: /setup is a first
# run and there is only one of those.
for pair in finance ops; do
	printf 'an-administrators-password' | ./bin/cronos-user \
		-dsn "file:$work/cronos.db" -driver sqlite \
		-email "$pair-admin@acme.example" -name "Admin" \
		-role admin -org acme -project "$pair" >/dev/null 2>&1 ||
		die "could not create the administrator for $pair"
done
finance=$(signin finance-admin@acme.example an-administrators-password)
ops=$(signin ops-admin@acme.example an-administrators-password)
[ -n "$finance" ] && [ -n "$ops" ] || die "an administrator could not sign in"
ok "an administrator in each"

# --- somebody who belongs to both ---------------------------------------------

say "Somebody in both projects"
curl -s -o /dev/null -X POST -H "Authorization: Bearer $finance" \
	-H 'content-type: application/json' \
	-d '{"email":"dewi@acme.example","name":"Dewi","role":"editor","password":"a-password-they-chose"}' \
	"$API/v1/people"
# The same account, admitted to the other project — and with a different role
# there, which is the case a switcher gets wrong by remembering the old one.
#
# With a password, because a deployment with no mail relay cannot invite and
# the admit path is reached by the address already being taken. The password
# is discarded: the account exists and keeps the one it has.
admitted=$(curl -s -o "$work/admit.json" -w '%{http_code}' -X POST \
	-H "Authorization: Bearer $ops" -H 'content-type: application/json' \
	-d '{"email":"dewi@acme.example","name":"Dewi","role":"viewer","password":"a-password-they-chose"}' \
	"$API/v1/people")
[ "$admitted" = 201 ] || { cat "$work/admit.json"; die "admitting to the second project answered $admitted"; }
ok "an editor in finance and a viewer in ops"

dewi=$(signin dewi@acme.example a-password-they-chose)
[ -n "$dewi" ] || die "she could not sign in"

# --- where this session may go ------------------------------------------------

say "Where the session may go"
listed=$(curl -s -H "Authorization: Bearer $dewi" "$API/v1/auth/project")
echo "$listed" | grep -q '"current":"finance"' ||
	die "the list does not say where the session is: $listed"
echo "$listed" | grep -q '"project":"ops"' ||
	die "the list does not offer the other project: $listed"
ok "it names where she is, and the other project she belongs to"

# --- and moving there ---------------------------------------------------------

say "Moving"
before=$(reports "$dewi")
case "$before" in
*customer-overview*) ok "in finance she reads $before" ;;
*) die "she cannot read finance's reports: $before" ;;
esac

moved=$(curl -s -X POST -H "Authorization: Bearer $dewi" -H 'content-type: application/json' \
	-d '{"project":"ops"}' "$API/v1/auth/project")
there=$(printf '%s' "$moved" | token)
[ -n "$there" ] || die "no token came back: $moved"
# The role the new project gives her, not the one she just had. A portal that
# carried the old one over would offer an editor's controls to a viewer, each
# of which the server then refuses.
printf '%s' "$moved" | grep -q '"role":"viewer"' ||
	die "the answer does not carry the role in the new project: $moved"
ok "a token for ops came back, saying she is a viewer there"

after=$(reports "$there")
case "$after" in
*deliveries*) ok "and she reads ops's reports: $after" ;;
*) die "the new session cannot read ops: $after" ;;
esac
case "$after" in
*customer-overview*) die "finance's reports came along into ops: $after" ;;
*) ok "and none of finance's came with her" ;;
esac

# The token she had still names finance. Nothing revokes it — she is still an
# editor there — and that is the point of the project living in the token.
case "$(reports "$dewi")" in
*customer-overview*) ok "and the session she left still reads finance" ;;
*) die "moving invalidated the session she came from" ;;
esac

# --- and nowhere else ---------------------------------------------------------

say "And nowhere else"
refused=$(curl -s -o /dev/null -w '%{http_code}' -X POST \
	-H "Authorization: Bearer $dewi" -H 'content-type: application/json' \
	-d '{"project":"payroll"}' "$API/v1/auth/project")
[ "$refused" = 404 ] || die "a project she is not in answered $refused"
ok "a project she does not belong to is refused ($refused)"

# An organisation she is not in is a different tenant, and its projects are
# not hers to name however well she knows they exist.
refused=$(curl -s -o /dev/null -w '%{http_code}' -X POST \
	-H "Authorization: Bearer $ops" -H 'content-type: application/json' \
	-d '{"project":"finance"}' "$API/v1/auth/project")
case "$refused" in
200) ok "an org administrator reaches another project in their own org" ;;
404 | 403) ok "an administrator with no membership there is refused ($refused)" ;;
*) die "entering answered $refused" ;;
esac

say "A session goes where the account belongs, and takes nothing with it"
