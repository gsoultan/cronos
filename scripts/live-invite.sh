#!/usr/bin/env bash
#
# Invites somebody, reads the email, and uses the link.
#
# The parts of this that can be wrong are all at boundaries a unit test owns
# both sides of: whether the mail actually leaves, whether the link in it is
# well-formed after passing through an SMTP body, whether the secret survives
# being URL-escaped, and whether the account that comes out is in the right
# project with the right role.
#
# So this runs a real SMTP server (MailHog), a real cronos against a real
# database, and then behaves like the person who got the email.
#
#   ./scripts/live-invite.sh
#
# Needs podman and go.
set -euo pipefail

cd "$(dirname "$0")/.."

PORT=8793
API="http://localhost:$PORT"
PORTAL="http://localhost:8794"
MAIL_SMTP=1025
MAIL_HTTP=8025

work=$(mktemp -d)
cleanup() {
	rm -rf "$work" || true
	[ -n "${server:-}" ] && { kill "$server" 2>/dev/null || true; }
	[ -n "${started_mail:-}" ] && podman rm -f cronos-mail >/dev/null 2>&1
	return 0
}
trap 'cleanup 2>/dev/null' EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok() { printf '  \033[32mok\033[0m %s\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

# --- a mail server ------------------------------------------------------------

say "Mail"
if ! curl -sf "http://localhost:$MAIL_HTTP/api/v2/messages" >/dev/null 2>&1; then
	started_mail=yes
	podman rm -f cronos-mail >/dev/null 2>&1 || true
	podman run -d --name cronos-mail \
		-p "$MAIL_SMTP:1025" -p "$MAIL_HTTP:8025" mailhog/mailhog >/dev/null
	for _ in $(seq 1 40); do
		curl -sf "http://localhost:$MAIL_HTTP/api/v2/messages" >/dev/null 2>&1 && break
		sleep 0.5
	done
fi
curl -sf "http://localhost:$MAIL_HTTP/api/v2/messages" >/dev/null || die "no mail server"
curl -s -X DELETE "http://localhost:$MAIL_HTTP/api/v1/messages" >/dev/null
ok "listening on $MAIL_SMTP"

# --- cronos -------------------------------------------------------------------

say "cronos"
go build -o bin/cronosd ./cmd/cronosd || die "build"
mkdir -p "$work/defs"

CRONOS_ADDR=":$PORT" \
	CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=sqlite \
	CRONOS_STORE_DSN="file:$work/cronos.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_ADMIN_KEY=admin-key-for-a-local-test \
	CRONOS_PORTAL_URL="$PORTAL" \
	CRONOS_SMTP_HOST="localhost:$MAIL_SMTP" \
	CRONOS_SMTP_FROM="cronos@acme.example" \
	./bin/cronosd >"$work/server.log" 2>&1 &
server=$!

for _ in $(seq 1 40); do
	curl -sf "$API/v1/health" >/dev/null 2>&1 && break
	sleep 0.25
done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/server.log"; die "cronos never came up"; }
ok "listening on $PORT with a mail server and a database"

# An administrator to do the inviting.
# The password comes over stdin, never a flag: a password on a command line is
# in the shell history and in every `ps` on the box.
printf 'an-administrators-password' | go run ./cmd/cronos-user \
	-dsn "file:$work/cronos.db" -driver sqlite \
	-email ada@acme.example -role admin -org default -project default >/dev/null 2>&1 ||
	die "could not create the first administrator"

session=$(curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","password":"an-administrators-password"}' |
	sed 's/.*"token":"//; s/".*//')
[ -n "$session" ] || die "could not sign in as the administrator"
ok "signed in as ada@acme.example"

# --- inviting -----------------------------------------------------------------

say "Inviting"

# No password field: that is what makes it an invitation rather than an account.
out=$(curl -s -w '\n%{http_code}' -X POST "$API/v1/people" \
	-H "Authorization: Bearer $session" -H 'content-type: application/json' \
	-d '{"email":"dewi@acme.example","name":"Dewi","role":"editor"}')
code=$(printf '%s' "$out" | tail -1)
[ "$code" = 201 ] || { printf '  %s\n' "$out"; die "inviting answered $code"; }
ok "the server accepted the invitation"

# Nobody can sign in as them yet: an invitation is not an account.
code=$(curl -s -o /dev/null -w '%{http_code}' "$API/v1/auth/login" \
	-H 'content-type: application/json' \
	-d '{"email":"dewi@acme.example","password":"anything-at-all"}')
[ "$code" != 200 ] || die "an invitation created a working account"
ok "no account exists yet"

# --- the email ----------------------------------------------------------------

say "The email"

for _ in $(seq 1 20); do
	count=$(curl -s "http://localhost:$MAIL_HTTP/api/v2/messages" |
		sed 's/.*"total":\([0-9]*\).*/\1/')
	[ "${count:-0}" -ge 1 ] && break
	sleep 0.25
done
[ "${count:-0}" -ge 1 ] || die "no email arrived"
ok "arrived at the mail server"

# The body as the recipient's client would render it: MailHog stores it
# quoted-printable, so soft line breaks have to be joined before the link is
# whole again. That unfolding is exactly what a mail client does, and doing it
# here is what proves the link survives transport.
body=$(curl -s "http://localhost:$MAIL_HTTP/api/v2/messages" |
	python3 -c '
import json, quopri, sys
message = json.load(sys.stdin)["items"][0]
raw = message["Content"]["Body"]
print(quopri.decodestring(raw.encode()).decode("utf-8", "replace"))')

printf '%s' "$body" | grep -q 'Dewi' || { printf '%s\n' "$body"; die "the email does not name them"; }
printf '%s' "$body" | grep -q 'ada@acme.example' || die "the email does not say who invited them"
ok "names them, and who invited them"

link=$(printf '%s' "$body" | grep -o "$PORTAL/invitation#secret=[A-Za-z0-9_-]*" | head -1)
[ -n "$link" ] || { printf '%s\n' "$body"; die "no link in the email"; }
secret=${link#*#secret=}
ok "carries a link with the secret in the fragment"

# The secret must not be anywhere a server would see it.
case "$link" in
*"?"*) die "the link has a query string: $link" ;;
esac
printf '%s' "$body" | grep -q "invitation?secret=" && die "the secret is in a query string"
ok "and nowhere a server would log it"

# --- accepting ----------------------------------------------------------------

say "Accepting"

described=$(curl -s "$API/v1/auth/invitation?secret=$secret")
printf '%s' "$described" | grep -q 'dewi@acme.example' ||
	{ printf '  %s\n' "$described"; die "the invitation does not describe itself"; }
printf '%s' "$described" | grep -q '"role":"editor"' || die "the role is wrong"
ok "describes itself without a session"

# A body that tries to say more than a secret and a password is refused, rather
# than having the extra fields ignored.
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/v1/auth/invitation" \
	-H 'content-type: application/json' \
	-d "{\"secret\":\"$secret\",\"password\":\"a-password-they-chose\",\"role\":\"admin\"}")
[ "$code" = 400 ] || die "a body naming a role answered $code"
ok "a body that names a role is refused"

accepted=$(curl -s -X POST "$API/v1/auth/invitation" -H 'content-type: application/json' \
	-d "{\"secret\":\"$secret\",\"password\":\"a-password-they-chose\"}")

printf '%s' "$accepted" | grep -q '"token"' ||
	{ printf '  %s\n' "$accepted"; die "no session came back"; }
printf '%s' "$accepted" | grep -q '"role":"editor"' || die "arrived with the wrong role"
ok "accepted, and signed in without a second login"

theirs=$(printf '%s' "$accepted" | sed 's/.*"token":"//; s/".*//')
code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $theirs" "$API/v1/catalog")
[ "$code" = 200 ] || die "the session they were handed does not work: $code"
ok "the session works"

# The password they chose is the one that signs them in.
code=$(curl -s -o /dev/null -w '%{http_code}' "$API/v1/auth/login" \
	-H 'content-type: application/json' \
	-d '{"email":"dewi@acme.example","password":"a-password-they-chose"}')
[ "$code" = 200 ] || die "the password they set does not sign them in: $code"
ok "the password is one only they have seen"

# --- once, and only once ------------------------------------------------------

say "Once"

code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/v1/auth/invitation" \
	-H 'content-type: application/json' \
	-d "{\"secret\":\"$secret\",\"password\":\"somebody-elses-password\"}")
[ "$code" = 410 ] || die "the link worked a second time: $code"
ok "the link does not work twice"

# And the second attempt did not change their password.
code=$(curl -s -o /dev/null -w '%{http_code}' "$API/v1/auth/login" \
	-H 'content-type: application/json' \
	-d '{"email":"dewi@acme.example","password":"somebody-elses-password"}')
[ "$code" != 200 ] || die "a replayed link changed their password"
ok "and replaying it changes nothing"

# An unknown secret answers exactly the same, so the endpoint cannot be used to
# learn which invitations are outstanding.
spent=$(curl -s -o /dev/null -w '%{http_code}' "$API/v1/auth/invitation?secret=$secret")
unknown=$(curl -s -o /dev/null -w '%{http_code}' "$API/v1/auth/invitation?secret=nobody-issued-this")
[ "$spent" = "$unknown" ] || die "spent answers $spent, unknown answers $unknown"
ok "a spent link and an invented one answer alike ($spent)"

say "All of it worked."
