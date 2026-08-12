#!/usr/bin/env bash
#
# Turning on "everybody needs a second factor", and what happens to the people
# who have none.
#
# That question is the whole feature. The flag is a column; what took deciding is
# that somebody without a factor cannot enrol without signing in and cannot sign
# in without enrolling — so refusing the sign-in locks a team out of its own
# reporting on the afternoon it is switched on, and puts an administrator on the
# phone being asked to turn a second factor off, which is the exact call a second
# factor exists to make suspicious.
#
# The answer is a session that reaches the enrolment routes and nothing else.
# This drives it: the requirement goes on while somebody is signed in without a
# factor, they sign in again, and every door is shut but one.
#
#   ./scripts/live-require-2fa.sh
#
# Needs go and python3. Leaves nothing behind.
set -euo pipefail

cd "$(dirname "$0")/.."

PORT=8802
API="http://localhost:$PORT"

work=$(mktemp -d)
cleanup() { [ -n "${srv:-}" ] && kill "$srv" 2>/dev/null; rm -rf "$work" || true; return 0; }
trap 'cleanup 2>/dev/null' EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok() { printf '  \033[32mok\033[0m %s\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

token() { sed 's/.*"token":"//; s/".*//'; }
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }

base=$(($(date +%s) / 30))
totp() {
	python3 - "$1" "$(($base + ${2:-0}))" <<-'PY'
		import base64, hashlib, hmac, struct, sys
		secret, counter = sys.argv[1], int(sys.argv[2])
		key = base64.b32decode(secret + "=" * (-len(secret) % 8))
		d = hmac.new(key, struct.pack(">Q", counter), hashlib.sha1).digest()
		o = d[-1] & 0x0F
		print(f"{(struct.unpack('>I', d[o:o+4])[0] & 0x7FFFFFFF) % 1000000:06d}")
	PY
}

say "A project with two people and no requirement"
go build -o bin/cronosd ./cmd/cronosd || die "build"
mkdir -p "$work/defs"

CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_STORE_DRIVER=sqlite CRONOS_STORE_DSN="file:$work/c.db" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	./bin/cronosd >"$work/log" 2>&1 &
srv=$!
for _ in $(seq 1 40); do curl -sf "$API/v1/health" >/dev/null 2>&1 && break; sleep 0.25; done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/log"; die "cronos never came up"; }

curl -s -X POST -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","name":"Ada","password":"a-password-they-chose","org":"acme","project":"finance"}' \
	"$API/v1/setup" >/dev/null
ada=$(curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","password":"a-password-they-chose"}' | token)
[ -n "$ada" ] || die "the administrator cannot sign in"

curl -s -X POST -H "Authorization: Bearer $ada" -H 'content-type: application/json' \
	-d '{"email":"dewi@acme.example","name":"Dewi","role":"editor","password":"a-password-for-dewi"}' \
	"$API/v1/people" >/dev/null
ok "ada administers it, dewi edits, neither has a second factor"

[ "$(code -H "Authorization: Bearer $ada" "$API/v1/catalog")" = 200 ] || die "ada cannot read her own project"
ok "and both can reach the project"

# --- turning it on ---------------------------------------------------------------

say "Turning the requirement on"

counted=$(curl -s -H "Authorization: Bearer $ada" "$API/v1/policy")
case "$counted" in *'"uncovered":2'*) ;; *) printf '  %s\n' "$counted"; die "the count is wrong" ;; esac
ok "it says two people have none, before anything changes"

curl -s -X PUT -H "Authorization: Bearer $ada" -H 'content-type: application/json' \
	-d '{"requireTwoFactor":true}' "$API/v1/policy" >/dev/null
case "$(curl -s -H "Authorization: Bearer $ada" "$API/v1/policy")" in
*'"requireTwoFactor":true'*) ;;
*) die "the requirement did not stick" ;;
esac
ok "the requirement is on"

# The session ada already holds is untouched. The requirement bites at sign-in,
# which is what stops turning it on from ejecting whoever turned it on.
[ "$(code -H "Authorization: Bearer $ada" "$API/v1/catalog")" = 200 ] ||
	die "turning it on ejected the person who turned it on"
ok "and the session that turned it on still works"

# --- signing in without one --------------------------------------------------------

say "Signing in without a second factor"

answer=$(curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"dewi@acme.example","password":"a-password-for-dewi"}')
dewi=$(printf '%s' "$answer" | token)
[ -n "$dewi" ] && [ "$dewi" != "$answer" ] || { printf '  %s\n' "$answer"; die "dewi cannot sign in at all"; }
ok "she signs in — she is not locked out"

case "$answer" in *'"mustEnrol":true'*) ;; *) printf '  %s\n' "$answer"; die "the portal is not told to show the wizard" ;; esac
ok "and is told the session may only enrol"

for path in /v1/catalog /v1/reports/monthly /v1/runs /v1/people /v1/policy /v1/auth/password; do
	got=$(code -H "Authorization: Bearer $dewi" "$API$path")
	[ "$got" = 403 ] || die "$path answered $got to a session that may only enrol"
done
ok "every other route is shut"

[ "$(code -X POST -H "Authorization: Bearer $dewi" "$API/v1/auth/factor/start")" = 200 ] ||
	die "she cannot start enrolling, which leaves her nowhere at all"
ok "and the enrolment routes are open"

# --- enrolling ---------------------------------------------------------------------

say "Enrolling, and the door opening"

started=$(curl -s -X POST -H "Authorization: Bearer $dewi" "$API/v1/auth/factor/start")
secret=$(printf '%s' "$started" | sed 's/.*"secret":"//; s/".*//')
[ -n "$secret" ] || { printf '  %s\n' "$started"; die "no secret"; }

confirmed=$(curl -s -X POST -H "Authorization: Bearer $dewi" -H 'content-type: application/json' \
	-d "{\"code\":\"$(totp "$secret")\"}" "$API/v1/auth/factor/confirm")
case "$confirmed" in *recoveryCodes*) ;; *) printf '  %s\n' "$confirmed"; die "enrolment failed" ;; esac
ok "a real code enrols her"

upgraded=$(printf '%s' "$confirmed" | token)
[ -n "$upgraded" ] && [ "$upgraded" != "$confirmed" ] ||
	die "no unrestricted session came back, so she must sign in again immediately"
ok "and the session she is holding becomes an ordinary one"

[ "$(code -H "Authorization: Bearer $upgraded" "$API/v1/catalog")" = 200 ] ||
	die "the upgraded session still cannot reach the project"
ok "which reaches the project"

# The old restricted token is not upgraded with it: it is a different string and
# still says what it said.
[ "$(code -H "Authorization: Bearer $dewi" "$API/v1/catalog")" = 403 ] ||
	die "the restricted token was somehow widened"
ok "while the restricted one it replaced still opens nothing"

# --- and next time -----------------------------------------------------------------

say "Next time she signs in"

again=$(curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
	-d '{"email":"dewi@acme.example","password":"a-password-for-dewi"}')
case "$again" in *'"factorRequired":true'*) ;; *) printf '  %s\n' "$again"; die "she is not asked for a code" ;; esac
ok "she is asked for a code, like anybody with a factor"

full=$(curl -s "$API/v1/auth/login" -H 'content-type: application/json' \
	-d "{\"email\":\"dewi@acme.example\",\"password\":\"a-password-for-dewi\",\"code\":\"$(totp "$secret" 1)\"}" | token)
[ -n "$full" ] || die "the code did not sign her in"
[ "$(code -H "Authorization: Bearer $full" "$API/v1/catalog")" = 200 ] ||
	die "the session is still restricted after enrolling"
ok "and gets an ordinary session"

say "All of it worked."
