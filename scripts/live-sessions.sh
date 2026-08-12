#!/usr/bin/env bash
#
# Signs in twice, loses one, and presses the button.
#
# The mechanism is a timestamp compared against a claim with second
# granularity, and the failure it had was invisible to a unit test written by
# the same reasoning that produced the bug: two browsers signed in moments
# apart land in one second, and the comparison that has to spare the
# replacement spared the lost session too. Two real sessions found it.
#
#   ./scripts/live-sessions.sh
#
# Needs go. Leaves nothing behind.
cd "$(dirname "$0")/.."
PORT=8795; API="http://localhost:$PORT"
work=$(mktemp -d)
trap 'rm -rf "$work"; [ -n "${srv:-}" ] && kill "$srv" 2>/dev/null; true' EXIT
ok(){ printf '  \033[32mok\033[0m %s\n' "$*"; }
die(){ printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

go build -o bin/cronosd ./cmd/cronosd
mkdir -p "$work/defs"
CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
  CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
  CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
  ./bin/cronosd >"$work/log" 2>&1 & srv=$!
for _ in $(seq 1 40); do curl -sf "$API/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/log"; die "no server"; }

printf 'an-administrators-password' | go run ./cmd/cronos-user \
  -dsn "file:$work/c.db" -driver sqlite -email ada@acme.example \
  -role admin -org default -project default >/dev/null 2>&1 || die "no admin"

login(){ curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
  -d '{"email":"ada@acme.example","password":"an-administrators-password"}' |
  sed 's/.*"token":"//; s/".*//'; }

# Two browsers, signed in a second apart — which is what put them in the
# same second and found the bug.
laptop=$(login); sleep 1; phone=$(login)
[ -n "$laptop" ] && [ -n "$phone" ] || die "could not sign in twice"
works(){ [ "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $1" "$API/v1/catalog")" = 200 ]; }
works "$laptop" && works "$phone" || die "a fresh session does not work"
ok "two sessions, both working"

# The phone is lost. From the laptop, end everything else.
out=$(curl -s -X POST -H "Authorization: Bearer $laptop" "$API/v1/auth/sessions/end")
replacement=$(printf '%s' "$out" | sed 's/.*"token":"//; s/".*//')
[ -n "$replacement" ] && [ "$replacement" != "$out" ] || { printf '  %s\n' "$out"; die "no replacement session"; }
ok "the server handed this browser a new session"

# The standing answer is cached for five seconds, which is the same trade
# already made for disabling somebody.
sleep 6
works "$phone" && die "the lost phone still works"
ok "the lost phone is signed out"
works "$laptop" && die "the old laptop token still works"
ok "and so is the token that pressed it"
works "$replacement" || die "the replacement does not work"
ok "but the browser that pressed it is still signed in"

# Pressing it again is fine, and the newest token survives.
again=$(curl -s -X POST -H "Authorization: Bearer $replacement" "$API/v1/auth/sessions/end" |
  sed 's/.*"token":"//; s/".*//')
sleep 6
works "$again" || die "pressing it twice signed everybody out"
ok "pressing it again is safe"
printf '\n\033[1mAll of it worked.\033[0m\n'
