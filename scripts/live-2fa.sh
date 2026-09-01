#!/usr/bin/env bash
#
# Enrols a second factor, computes a code the way an authenticator app would,
# and signs in with it.
#
# The point of computing the code independently — from the otpauth:// URI, with
# a TOTP implementation that is not the one under test — is that it is the only
# way to know an app would agree. A test that checks cronos against cronos
# passes just as well when both halves are wrong, which is precisely how the
# wizard this replaces shipped: it accepted any six digits, and the QR beside it
# encoded nothing at all.
#
#   ./scripts/live-2fa.sh
#
# Needs go and python3. Leaves nothing behind.
set -euo pipefail

cd "$(dirname "$0")/.."

PORT=8796
API="http://localhost:$PORT"

work=$(mktemp -d)
cleanup() {
	rm -rf "$work" || true
	[ -n "${srv:-}" ] && { kill "$srv" 2>/dev/null || true; }
	return 0
}
trap 'cleanup 2>/dev/null' EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok() { printf '  \033[32mok\033[0m %s\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

# An independent TOTP, from the RFC rather than from internal/core/identity.
#
# Against a base step captured once, not against the wall clock at each call:
# every accepted code advances the line that the next one is checked against, so
# these assertions only mean anything if the script controls which step each one
# comes from. The whole run takes about two seconds, well inside the window the
# server accepts around its own clock.
base=$(($(date +%s) / 30))

totp() {
	python3 - "$1" "$(($base + ${2:-0}))" <<-'PY'
		import base64, hashlib, hmac, struct, sys
		secret, counter = sys.argv[1], int(sys.argv[2])
		key = base64.b32decode(secret + "=" * (-len(secret) % 8))
		digest = hmac.new(key, struct.pack(">Q", counter), hashlib.sha1).digest()
		offset = digest[-1] & 0x0F
		code = struct.unpack(">I", digest[offset:offset + 4])[0] & 0x7FFFFFFF
		print(f"{code % 1000000:06d}")
	PY
}

say "cronos"
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

printf 'a-password-they-chose' | go run ./cmd/cronos-user \
	-dsn "file:$work/c.db" -driver sqlite -email ada@acme.example \
	-role admin -org default -project default >/dev/null 2>&1 || die "no account"

login() {
	body='{"email":"ada@acme.example","password":"a-password-they-chose"'
	[ -n "${1:-}" ] && body="$body,\"code\":\"$1\""
	curl -s "$API/v1/auth/login" -H 'content-type: application/json' -d "$body}"
}
session=$(login | sed 's/.*"token":"//; s/".*//')
[ -n "$session" ] || die "could not sign in"
ok "signed in with a password alone"

# --- enrolling ----------------------------------------------------------------

say "Enrolling"

started=$(curl -s -X POST -H "Authorization: Bearer $session" "$API/v1/auth/factor/start")
secret=$(printf '%s' "$started" | sed 's/.*"secret":"//; s/".*//')
uri=$(printf '%s' "$started" | sed 's/.*"uri":"//; s/".*//' | sed 's/\\u0026/\&/g')
[ -n "$secret" ] || { printf '  %s\n' "$started"; die "no secret"; }
ok "the server minted a secret"

case "$uri" in
otpauth://totp/*) ok "and an otpauth:// URI an app can scan" ;;
*) die "the URI is $uri" ;;
esac

# The URI has to carry the same secret, or the phone and the server hold
# different keys and enrolment can never be confirmed.
printf '%s' "$uri" | grep -q "secret=$secret" || die "the QR and the stored secret disagree"
ok "carrying the same secret the server stored"

# Nothing is protected yet: an enrolment that was never proved must not count.
printf '%s' "$(login)" | grep -q '"token"' || die "an unproved enrolment already blocks sign-in"
ok "and nothing is protected until it is proved"

# A wrong code does not confirm it — the assertion the old wizard could not make.
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $session" \
	-H 'content-type: application/json' -d '{"code":"000000"}' "$API/v1/auth/factor/confirm")
[ "$code" != 200 ] || die "any six digits confirmed the enrolment"
ok "a wrong code does not confirm it"

# The real one, computed independently.
confirmed=$(curl -s -X POST -H "Authorization: Bearer $session" -H 'content-type: application/json' \
	-d "{\"code\":\"$(totp "$secret")\"}" "$API/v1/auth/factor/confirm")
printf '%s' "$confirmed" | grep -q recoveryCodes ||
	{ printf '  %s\n' "$confirmed"; die "a code an app would produce was refused"; }
ok "a code computed the way an app would is accepted"

recovery=$(printf '%s' "$confirmed" |
	python3 -c 'import json,sys; print("\n".join(json.load(sys.stdin)["recoveryCodes"]))')
[ "$(printf '%s\n' "$recovery" | wc -l | tr -d ' ')" = 10 ] || die "no recovery codes"
ok "ten recovery codes came back"

# --- signing in ---------------------------------------------------------------

say "Signing in"

answer=$(login)
printf '%s' "$answer" | grep -q '"token"' && die "the password alone still signs in"
printf '%s' "$answer" | grep -q '"factorRequired":true' ||
	{ printf '  %s\n' "$answer"; die "the portal is not told to ask for a code"; }
ok "the password alone is no longer enough"

# The order of what follows is the point, and it took a live run to see it.
#
# Two rules meet here. The drift window accepts a code from one step either
# side of the server's clock; replay protection accepts only a code newer than
# the last one that got in. Confirming the enrolment already used the code at
# the base step, so that line now sits at base — and everything at or below it
# is refused however good the arithmetic.
#
# That is right, and it is not what a unit test written from the algorithm
# would have expected. On an account used once a day the line is hours old and
# drift in either direction works; immediately after a sign-in, only forward
# does.

# The code enrolment spent. Refused, and reported as spent rather than wrong.
used=$(login "$(totp "$secret" 0)")
printf '%s' "$used" | grep -q '"token"' && die "the code that confirmed enrolment signed in too"
printf '%s' "$used" | grep -q "already been used" ||
	{ printf '  %s\n' "$used"; die "a spent code is not reported as one"; }
ok "the code that confirmed the enrolment cannot sign in"

# One from behind the line. Inside the drift window, and still refused.
printf '%s' "$(login "$(totp "$secret" -1)")" | grep -q '"token"' &&
	die "a code older than the last one accepted was let through"
ok "and neither can one from behind it"

# A phone whose clock runs fast, which is the drift the window exists for.
printf '%s' "$(login "$(totp "$secret" 1)")" | grep -q '"token"' ||
	die "a code from the next step was refused"
ok "a code from a clock thirty seconds fast signs in"

# And not twice.
replayed=$(login "$(totp "$secret" 1)")
printf '%s' "$replayed" | grep -q '"token"' && die "the same code signed in twice"
printf '%s' "$replayed" | grep -q "already been used" ||
	{ printf '  %s\n' "$replayed"; die "a replayed code is not reported as one"; }
ok "and does not work a second time"

# A recovery code, once. The session it returns is kept: every TOTP step this
# run can reach has been spent by now, and waiting thirty seconds for the next
# one would be waiting for the clock rather than testing anything.
first=$(printf '%s\n' "$recovery" | head -1)
signed=$(login "$first")
session=$(printf '%s' "$signed" | sed 's/.*"token":"//; s/".*//')
[ -n "$session" ] && [ "$session" != "$signed" ] ||
	{ printf '  %s\n' "$signed"; die "a recovery code did not sign in"; }
ok "a recovery code signs in"

printf '%s' "$(login "$first")" | grep -q '"token"' && die "a recovery code worked twice"
ok "and is spent"

# --- turning it off -----------------------------------------------------------

say "Turning it off"

# Twelve sign-ins in two seconds is not what a person does, and the per-address
# limit on this route says so — correctly. It is five a minute with ten in hand
# and, unlike the per-account one, nothing resets it, because the attack it
# exists for is one machine working through a list. Waiting for it to refill is
# the honest way past it: raising the limit for the test would be testing a
# server nobody runs.
printf '  waiting %ss for the sign-in limit to refill\n' 30
sleep 30


curl -s -o /dev/null -X DELETE -H "Authorization: Bearer $session" \
	-H 'content-type: application/json' -d '{"code":"000000"}' "$API/v1/auth/factor"

# Asserted by what the account still requires rather than by the status code: a
# 401 for a bad session and a 401 for a bad code look the same from out here,
# and only one of them would mean the check worked.
printf '%s' "$(login)" | grep -q '"factorRequired":true' ||
	die "a guessed code turned off the second factor"
ok "a guessed code does not turn it off"

second=$(printf '%s\n' "$recovery" | sed -n 2p)
code=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE -H "Authorization: Bearer $session" \
	-H 'content-type: application/json' -d "{\"code\":\"$second\"}" "$API/v1/auth/factor")
[ "$code" = 204 ] || die "a recovery code did not turn it off: $code"
ok "a recovery code does"

printf '%s' "$(login)" | grep -q '"token"' || die "the account cannot sign in after removal"
ok "and the password alone works again"

# The recovery codes went with it: eight live credentials for an unprotected
# account is not a lesser risk than the factor was, it is a set of passwords
# nobody believes exist any more.
#
# Read from the table rather than inferred from a sign-in. With no factor the
# code field is ignored and the password signs in on its own — correct, and it
# means a sign-in cannot tell a deleted code from an accepted one.
rows() { python3 -c "import sqlite3,sys; print(sqlite3.connect(sys.argv[1]).execute('SELECT COUNT(*) FROM '+sys.argv[2]).fetchone()[0])" "$work/c.db" "$1"; }

[ "$(rows cronos_recovery_codes)" = 0 ] || die "recovery codes outlived the factor"
ok "the leftover recovery codes are gone"

[ "$(rows cronos_factors)" = 0 ] || die "the factor row survived removal"
ok "and so is the secret"

say "All of it worked."
