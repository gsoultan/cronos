#!/usr/bin/env bash
# Drives the portal against a real cronosd.
#
# Every other portal suite runs on sample data, which is what makes the
# interface workable before a server exists — and means none of them would
# notice if the API contract changed. This one would.
set -euo pipefail
cd "$(dirname "$0")/.."

export CRONOS_SIGNING_KEY="${CRONOS_SIGNING_KEY:-development-key-at-least-32-bytes-long}"
export CRONOS_DEFINITIONS=demo/definitions
export CRONOS_SEED=demo/seed.sql
export CRONOS_ADDR="${CRONOS_ADDR:-:8794}"
export CRONOS_ORG=acme CRONOS_PROJECT=finance
# Armed, so the catalogue can say when each schedule next fires.
export CRONOS_SCHEDULER=1
export CRONOS_DELIVERIES="${TMPDIR:-/tmp}/cronos-live-deliveries"
# Users live in the definition store, so sign-in needs one.
export CRONOS_STORE_DRIVER=sqlite
export CRONOS_STORE_DSN="file:${TMPDIR:-/tmp}/cronos-live-users.db"
rm -f "${TMPDIR:-/tmp}/cronos-live-users.db"
PORT="${CRONOS_ADDR#:}"
WEB_PORT="${PORTAL_PORT:-5174}"
export CRONOS_ORIGINS="http://localhost:${WEB_PORT}"

free_port() {
  if lsof -ti "tcp:$1" >/dev/null 2>&1; then
    lsof -ti "tcp:$1" | xargs kill 2>/dev/null || true
    sleep 1
  fi
}

for p in "$PORT" "$WEB_PORT"; do free_port "$p"; done

go build -o bin/cronosd ./cmd/cronosd
go build -o bin/cronos-token ./cmd/cronos-token

./bin/cronosd > /tmp/cronosd-portal.log 2>&1 &
SERVER=$!
trap 'kill $SERVER 2>/dev/null || true; free_port "$WEB_PORT"' EXIT

for _ in $(seq 1 40); do
  curl -sf -o /dev/null "http://localhost:${PORT}/v1/health" && break
  sleep 0.25
done

go build -o bin/cronos-user ./cmd/cronos-user
echo "correct horse battery staple" | ./bin/cronos-user \
  -email dewi@acme.example -name Dewi -org acme -project finance -role editor >/dev/null

TOKEN="$(./bin/cronos-token -audience portal -role editor -org acme -project finance -subject dewi)"

# The portal is built with the API baked in, the way a deployment would be.
( cd apps/portal && \
  VITE_CRONOS_API="http://localhost:${PORT}" VITE_CRONOS_TOKEN="$TOKEN" \
  bunx vite --port "$WEB_PORT" --strictPort > /tmp/portal-live.log 2>&1 ) &
WEB=$!

for _ in $(seq 1 60); do
  curl -sf -o /dev/null "http://localhost:${WEB_PORT}/" && break
  sleep 0.5
done

# API and token too: the edit check reads back what the form published, and
# asking the server directly is the only way to see the stored bytes.
# A viewer too, because "who may delete" is a rule with two answers and a
# check that only ever holds the permitted one proves half of it.
VIEWER="$(./bin/cronos-token -audience portal -role viewer -org acme -project finance -subject sam)"
BASE="http://localhost:${WEB_PORT}" API="http://localhost:${PORT}" TOKEN="$TOKEN" VIEWER="$VIEWER" \
  node apps/portal/scripts/live-portal-check.mjs

# And again with no token baked in, so the portal has to sign somebody in.
#
# Killed by port, not by job id: $WEB is the subshell, and vite is its child —
# killing the parent leaves the server listening, so the next instance loses
# --strictPort and the old one keeps serving the token-baked build.
free_port "$WEB_PORT"
( cd apps/portal && VITE_CRONOS_API="http://localhost:${PORT}" \
  bunx vite --port "$WEB_PORT" --strictPort > /tmp/portal-signin.log 2>&1 ) &
WEB=$!
for _ in $(seq 1 60); do
  curl -sf -o /dev/null "http://localhost:${WEB_PORT}/" && break
  sleep 0.5
done
BASE="http://localhost:${WEB_PORT}" node apps/portal/scripts/live-signin-check.mjs
