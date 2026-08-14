#!/usr/bin/env bash
#
# Drives a whole sign-in and sign-out against a real identity provider.
#
# Every part of SSO that can be wrong is wrong at a boundary: a redirect URL the
# provider does not recognise, a discovery document without the field we expected,
# a cookie the browser withholds, a logout the provider refuses because it wanted
# a hint we did not keep. Unit tests answer none of those, because both sides of
# each boundary are ours in a unit test.
#
# So this stands up Keycloak — the provider most self-hosted deployments run, and
# one that publishes an end_session_endpoint, which Dex does not — creates a
# realm, a client and a person, and then behaves like a browser: follows the
# redirects, keeps the cookies, and reads what comes back.
#
#   ./scripts/live-sso.sh
#
# Needs podman and a built cronosd-ee. Leaves nothing behind.
set -euo pipefail

cd "$(dirname "$0")/.."

KC=http://localhost:8088
REALM=cronos
CLIENT=cronos-portal
SECRET=a-client-secret-for-a-local-test
PERSON=dewi@acme.example
PASSWORD=hunter2-hunter2
PORT=8791
API="http://localhost:$PORT"
# The portal is a static build served separately from the API — that is how it
# is actually deployed — so the landing after a sign-out is somebody else's
# origin. A directory with one file stands in for it.
PORTAL_PORT=8792
PORTAL="http://localhost:$PORTAL_PORT"

work=$(mktemp -d)
jar="$work/cookies"

# Every step guarded, because a trap runs under `set -e` too: one command
# returning non-zero — killing a server that already died, say — aborts the
# trap and leaves the rest of the cleanup undone.
cleanup() {
	rm -rf "$work" || true
	[ -n "${server:-}" ] && kill "$server" 2>/dev/null
	[ -n "${portal:-}" ] && kill "$portal" 2>/dev/null
	# Only if this run started it. Keycloak takes two minutes to come up, and
	# tearing down one somebody else was using makes the next run of this
	# script slower for no reason.
	[ -n "${started_keycloak:-}" ] && podman rm -f cronos-kc >/dev/null 2>&1
	return 0
}
trap "cleanup 2>/dev/null" EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok() { printf '  \033[32mok\033[0m %s\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

# --- Keycloak -----------------------------------------------------------------

say "Keycloak"
if ! curl -sf "$KC/realms/master/.well-known/openid-configuration" >/dev/null 2>&1; then
	started_keycloak=yes
	podman rm -f cronos-kc >/dev/null 2>&1 || true
	podman run -d --name cronos-kc -p 8088:8080 \
		-e KC_BOOTSTRAP_ADMIN_USERNAME=admin -e KC_BOOTSTRAP_ADMIN_PASSWORD=admin \
		quay.io/keycloak/keycloak:26.0 start-dev >/dev/null
	for _ in $(seq 1 60); do
		curl -sf "$KC/realms/master/.well-known/openid-configuration" >/dev/null 2>&1 && break
		sleep 5
	done
fi
curl -sf "$KC/realms/master/.well-known/openid-configuration" >/dev/null || die "keycloak never came up"
ok "running"

kc() { podman exec cronos-kc /opt/keycloak/bin/kcadm.sh "$@"; }

kc config credentials --server http://localhost:8080 --realm master \
	--user admin --password admin >/dev/null 2>&1 || die "could not authenticate to keycloak"

have() { kc get "$1" -r "$REALM" -q "$2" --fields id --format csv --noquotes 2>/dev/null | grep -q .; }

kc get "realms/$REALM" >/dev/null 2>&1 || kc create realms -s "realm=$REALM" -s enabled=true >/dev/null
have clients "clientId=$CLIENT" || {
	kc create clients -r "$REALM" \
		-s "clientId=$CLIENT" -s enabled=true -s protocol=openid-connect \
		-s publicClient=false -s "secret=$SECRET" -s standardFlowEnabled=true \
		-s "redirectUris=[\"$API/v1/auth/sso/callback\"]" \
		-s "attributes.\"post.logout.redirect.uris\"=$PORTAL/*" >/dev/null
}
have users "email=$PERSON" || {
	kc create users -r "$REALM" -s "username=$PERSON" -s "email=$PERSON" \
		-s emailVerified=true -s firstName=Dewi -s lastName=Rahayu -s enabled=true >/dev/null
	id=$(kc get users -r "$REALM" -q "email=$PERSON" --fields id --format csv --noquotes | head -1)
	kc set-password -r "$REALM" --userid "$id" --new-password "$PASSWORD" >/dev/null
}
ok "realm $REALM, client $CLIENT, one person"

# The field this whole feature rests on. Worth asserting rather than assuming:
# a provider that stopped publishing it would silently turn single log-out off.
curl -s "$KC/realms/$REALM/.well-known/openid-configuration" |
	grep -q end_session_endpoint || die "this provider publishes no end_session_endpoint"
ok "publishes end_session_endpoint"

# --- cronos -------------------------------------------------------------------

say "cronos"
go build -o bin/cronosd-ee ./cmd/cronosd-ee || die "build"

# Nobody else's server, on either port. A leftover from a crashed run answers
# on the same port and is not this stub — which is how this check spent a while
# asserting things about somebody else's 404. Named rather than worked around:
# a check that quietly talks to the wrong server is worse than one that stops.
for port in "$PORT" "$PORTAL_PORT"; do
	if curl -s -o /dev/null --max-time 1 "http://localhost:$port/" 2>/dev/null; then
		die "something is already listening on $port — stop it and run this again"
	fi
done

mkdir -p "$work/portal"
printf '<!doctype html><title>cronos</title><h1>Signed out</h1>' >"$work/portal/index.html"
# --directory rather than a subshell that cds: the subshell's pid is what the
# cleanup would hold, and killing it leaves python orphaned on the port for
# every run after this one.
python3 -m http.server "$PORTAL_PORT" --directory "$work/portal" >/dev/null 2>&1 &
portal=$!
for _ in $(seq 1 40); do curl -sf -o /dev/null "$PORTAL/" && break; sleep 0.25; done
curl -sf -o /dev/null "$PORTAL/" || die "the portal stub did not come up"

mkdir -p "$work/defs"
CRONOS_ADDR=":$PORT" \
	CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_ADMIN_KEY=admin-key-for-a-local-test \
	CRONOS_OIDC_ISSUER="$KC/realms/$REALM" \
	CRONOS_OIDC_CLIENT_ID="$CLIENT" \
	CRONOS_OIDC_CLIENT_SECRET="$SECRET" \
	CRONOS_OIDC_REDIRECT_URL="$API/v1/auth/sso/callback" \
	CRONOS_OIDC_POST_LOGOUT_URL="$PORTAL/" \
	./bin/cronosd-ee >"$work/server.log" 2>&1 &
server=$!

for _ in $(seq 1 40); do
	curl -sf "$API/v1/health" >/dev/null 2>&1 && break
	sleep 0.25
done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/server.log"; die "cronos never came up"; }
ok "listening on $PORT with an OIDC provider registered"

# --- signing in ---------------------------------------------------------------

say "Signing in"

# A browser: keep the cookies, follow the redirects, and stop at the login form.
form=$(curl -s -c "$jar" -b "$jar" -L "$API/v1/auth/sso/start?returning=/reports")
action=$(printf '%s' "$form" | grep -o 'action="[^"]*"' | head -1 | sed 's/action="//; s/"$//' | sed 's/&amp;/\&/g')
[ -n "$action" ] || { printf '%s\n' "$form" | head -40; die "no login form came back"; }
ok "the provider showed its own login page"

# Submitting it lands back on /v1/auth/sso/callback, which answers with the
# fragment handoff. --max-redirs 0 so we can read that redirect rather than
# following it into the portal.
landing=$(curl -s -c "$jar" -b "$jar" -o /dev/null -w '%{redirect_url}' \
	--data-urlencode "username=$PERSON" --data-urlencode "password=$PASSWORD" \
	--data-urlencode "credentialId=" "$action")

# Then a few more hops — through the provider, back to /v1/auth/sso/callback,
# and out to the portal — one at a time, so the last one can be read rather than
# followed. A browser would do exactly this and stop when the fragment arrives.
for _ in $(seq 1 8); do
	case "$landing" in
	'' | *'#token='*) break ;;
	esac
	landing=$(curl -s -c "$jar" -b "$jar" -o /dev/null -w '%{redirect_url}' "$landing")
done

case "$landing" in
*'#token='*) ok "came back with a session in the fragment" ;;
*) printf '  landed on: %s\n' "$landing"; tail -20 "$work/server.log"; die "no session came back" ;;
esac

session=${landing#*'#token='}
[ -n "$session" ] || die "the fragment held no token"

# It landed where it was asked to, not somewhere else. An SSO callback that
# ignores where the sign-in started sends everybody to the home page, which is
# the small thing people notice every day.
case "${landing%%#*}" in
*/reports) ok "landed on /reports, where the sign-in started" ;;
*) die "landed on ${landing%%#*}, not /reports" ;;
esac

# The identity token must not be inside it. It is a credential at the provider,
# and the cronos session lives in localStorage where any script can read it.
if printf '%s' "$session" | base64 -d 2>/dev/null | grep -qi 'id_token\|eyJ'; then
	die "the identity token is inside the cronos session"
fi
ok "the identity token is not in the session"

# What the session says it is. A portal audience and a subject namespaced by
# the provider: an embed audience here would mean somebody signing in through a
# directory got a token for the wrong half of the product.
claims=$(printf '%s' "$session" | cut -d. -f2 | tr '_-' '/+' | base64 -d 2>/dev/null || true)
printf '%s' "$claims" | grep -q '"aud":"portal"' || { printf '  %s\n' "$claims"; die "not a portal session"; }
printf '%s' "$claims" | grep -q '"sub":"sso_oidc_' || { printf '  %s\n' "$claims"; die "the subject is not namespaced by the provider"; }
ok "a portal session, namespaced by the provider"

# And it works: the server accepts it on a route that requires one.
code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $session" "$API/v1/catalog")
[ "$code" = 200 ] || die "the session was refused by the server: $code"
ok "the server accepts it"

# --- signing out --------------------------------------------------------------

say "Signing out"

out=$(curl -s -X POST -H "Authorization: Bearer $session" "$API/v1/auth/sso/logout")
redirect=$(printf '%s' "$out" | sed 's/.*"redirect":"//; s/".*//; s/\\u0026/\&/g')

[ -n "$redirect" ] || { printf '  %s\n' "$out"; die "nowhere to send the browser"; }
case "$redirect" in
"$KC/realms/$REALM/protocol/openid-connect/logout"*) ok "goes to the provider's own end-session endpoint" ;;
*) die "goes to $redirect" ;;
esac

printf '%s' "$redirect" | grep -q 'id_token_hint=' || die "no id_token_hint — okta would refuse this"
printf '%s' "$redirect" | grep -q "client_id=$CLIENT" || die "no client_id"
ok "carries both an id_token_hint and a client_id"

# The hint must not be in the answer as anything but the hint — and the browser
# is about to put this URL in its address bar, which is the one place it is
# least secret. Sending it is the protocol; it is the copy in localStorage and
# the copy in the database that this design refuses.

# Now behave like the browser and actually go there. This is the assertion the
# unit tests cannot make: that the provider accepts what we built.
ended=$(curl -s -c "$jar" -b "$jar" -o "$work/logout.html" -w '%{http_code} %{url_effective}' -L "$redirect")
code=${ended%% *}
where=${ended#* }

case "$code" in
200 | 204) ok "the provider accepted it ($code)" ;;
# Named with where it ended up. A status on its own cannot distinguish the
# provider refusing the request from the portal stub not serving the page it
# was sent back to, and those are opposite problems.
*) head -30 "$work/logout.html"; die "logout ended at $where with $code" ;;
esac
if grep -qi 'invalid\|error' "$work/logout.html"; then
	head -20 "$work/logout.html"
	die "the provider's page says the request was invalid"
fi

# And it came back to the portal rather than stopping at the provider's own
# page — which is what CRONOS_OIDC_POST_LOGOUT_URL is for, and what an
# unregistered value would have broken.
case "$where" in
"$PORTAL"/*) ok "landed back on the portal" ;;
*) die "landed on $where" ;;
esac

# And the provider's session is genuinely over: starting a sign-in again shows
# the login form rather than waving the same person straight through. This is
# the whole point — before single log-out, this step signed them back in
# silently and the sign-out button looked broken.
again=$(curl -s -c "$jar" -b "$jar" -L "$API/v1/auth/sso/start?returning=/")
printf '%s' "$again" | grep -q 'action="[^"]*login-actions' ||
	{ printf '%s\n' "$again" | head -30; die "the next sign-in was silent — the provider's session survived"; }
ok "the next sign-in asks who you are"

# The hint is spent. A second sign-out gets the endpoint without one rather
# than a second copy of a credential.
second=$(curl -s -X POST -H "Authorization: Bearer $session" "$API/v1/auth/sso/logout")
printf '%s' "$second" | grep -q 'id_token_hint=' && die "the hint survived being spent"
ok "the hint was good for exactly one sign-out"

# Nothing in the request decides where this goes.
sneaky=$(curl -s -X POST -H "Authorization: Bearer $session" \
	"$API/v1/auth/sso/logout?returning=//evil.example&post_logout_redirect_uri=https://evil.example")
printf '%s' "$sneaky" | grep -q 'evil.example' && die "a request parameter reached the redirect"
ok "no request parameter reaches the redirect"

say "All of it worked."
