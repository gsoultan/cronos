#!/usr/bin/env bash
#
# Forgetting a password, and getting back in.
#
# Until this there was no way. cronos-user creates accounts and deliberately
# will not reset one, so the recovery path for the commonest support request in
# software was a shell on the server and a bcrypt hash written by hand — an
# outage for the person, and a standing reason for somebody to keep a
# production DSN on a laptop.
#
# Driven end to end because the parts that matter are the joins: a real mail
# server, a real link out of a real inbox, a real browser typing a new password,
# and a sign-in afterwards. What it asserts, in order:
#
#   1. an address nobody has answers exactly like one that exists
#   2. the mail arrives, and the secret is in the fragment rather than the query
#   3. the link sets a password, once, and says the old one has stopped working
#   4. every session that account had has ended, including somebody else's
#   5. and a second factor is still asked for, because a mailbox is not one
#
#   ./scripts/live-reset.sh
#
# Needs go, podman (for MailHog), bun and playwright's chromium. Leaves nothing
# behind.
set -euo pipefail
cd "$(dirname "$0")/.."

API_PORT=8811
PORTAL_PORT=5275
MAIL_SMTP=1027
MAIL_HTTP=8027
API="http://localhost:$API_PORT"
PORTAL="http://localhost:$PORTAL_PORT"

work=$(mktemp -d)
cleanup() {
	[ -n "${server:-}" ] && { kill "$server" 2>/dev/null || true; }
	freeport "$PORTAL_PORT"
	[ -n "${started_mail:-}" ] && { podman rm -f cronos-reset-mail >/dev/null 2>&1 || true; }
	rm -rf "$work" || true
	return 0
}
trap 'cleanup 2>/dev/null' EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok() { printf '  \033[32mok\033[0m %s\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

# lsof exits 1 when it finds nothing, which under `set -e` and `pipefail` ends
# the script silently — the failure is a pipeline rather than a command.
freeport() {
	local pids
	pids=$(lsof -ti ":$1" 2>/dev/null || true)
	[ -n "$pids" ] && { kill $pids 2>/dev/null || true; }
	return 0
}
inuse() { [ -n "$(lsof -ti ":$1" 2>/dev/null || true)" ]; }

command -v podman >/dev/null 2>&1 || { echo "skipped: needs podman for a mail server"; exit 0; }

say "Mail"
podman rm -f cronos-reset-mail >/dev/null 2>&1 || true
started_mail=yes
podman run -d --name cronos-reset-mail \
	-p "$MAIL_SMTP:1025" -p "$MAIL_HTTP:8025" mailhog/mailhog >/dev/null
for _ in $(seq 1 40); do
	curl -sf "http://localhost:$MAIL_HTTP/api/v2/messages" >/dev/null 2>&1 && break
	sleep 0.5
done
curl -sf "http://localhost:$MAIL_HTTP/api/v2/messages" >/dev/null || die "no mail server"
curl -s -X DELETE "http://localhost:$MAIL_HTTP/api/v1/messages" >/dev/null
ok "listening on $MAIL_SMTP"

say "cronos"
go build -o bin/cronosd ./cmd/cronosd || die "build"
inuse "$API_PORT" && die "something is already on $API_PORT"
mkdir -p "$work/defs"

CRONOS_ADDR=":$API_PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_ORIGINS="$PORTAL" CRONOS_PORTAL_URL="$PORTAL" \
	CRONOS_SMTP_HOST="localhost:$MAIL_SMTP" CRONOS_SMTP_FROM="cronos@acme.example" \
	./bin/cronosd >"$work/server.log" 2>&1 &
server=$!
for _ in $(seq 1 60); do curl -sf "$API/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/server.log"; die "cronos never came up"; }

curl -s -o /dev/null -X POST -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","name":"Ada","password":"the-password-she-forgot",
	     "org":"acme","project":"finance"}' "$API/v1/setup"
ok "on $API_PORT, with one account"

# The sign-in page has to be able to know whether to offer the link at all.
curl -s "$API/v1/auth/methods" | grep -q '"reset":true' ||
	die "the deployment does not advertise that a password can be reset"
ok "and it says a reset is possible, so the sign-in page can offer it"

# -- 1. Asking says nothing ---------------------------------------------------

say "Asking"
ask() {
	curl -s -X POST -H 'content-type: application/json' -d "{\"email\":\"$1\"}" \
		"$API/v1/auth/password/forgot"
}
known=$(ask ada@acme.example)
unknown=$(ask nobody@acme.example)
[ "$known" = "$unknown" ] ||
	die "a known address answers differently from an unknown one:
    known:   $known
    unknown: $unknown"
ok "a known address and an unknown one give the same answer, word for word"

sleep 2
count=$(curl -s "http://localhost:$MAIL_HTTP/api/v2/messages" |
	python3 -c 'import json,sys; print(json.load(sys.stdin)["total"])')
[ "$count" = 1 ] || die "$count messages were sent, and one address has an account"
ok "and exactly one message was sent, to the one that does"

# -- 2. What is in it ---------------------------------------------------------

link=$(curl -s "http://localhost:$MAIL_HTTP/api/v2/messages" | python3 -c '
import json, sys, quopri, re
d = json.load(sys.stdin)
body = quopri.decodestring(d["items"][0]["Content"]["Body"]).decode()
m = re.search(r"(http\S*/reset\S+)", body)
print(m.group(1) if m else "")')
[ -n "$link" ] || die "the mail has no link in it"

# The fragment, not the query string. A browser sends a query string to every
# proxy and access log between here and the page; it sends a fragment to none of
# them. A reset secret in `?secret=` is a working key to the account written
# into somebody's logs.
case "$link" in
*"/reset#secret="*) ok "the link carries its secret in the fragment, which no server sees" ;;
*"?secret="*) die "the secret is in the query string, so it reaches every log on the way" ;;
*) die "the link is not shaped like a reset: $link" ;;
esac

say "portal"
freeport "$PORTAL_PORT"
sleep 1
(cd apps/portal && VITE_CRONOS_API="$API" bun run dev --port "$PORTAL_PORT" \
	>"$work/portal.log" 2>&1) &
for _ in $(seq 1 60); do curl -sf "$PORTAL/" >/dev/null 2>&1 && break; sleep 0.5; done
curl -sf "$PORTAL/" >/dev/null || { tail -20 "$work/portal.log"; die "the portal never came up"; }
ok "on $PORTAL_PORT"

# -- 3, 4, 5. Through a browser ------------------------------------------------

say "Getting back in, in a browser"
(cd apps/portal && PORTAL="$PORTAL" API="$API" LINK="$link" node e2e/reset.mjs) ||
	die "the browser check"
