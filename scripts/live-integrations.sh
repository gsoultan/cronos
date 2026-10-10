#!/usr/bin/env bash
# Runs the two example host applications in integrations/ — plain JavaScript
# and React — in a real browser against a real cronosd.
#
# What it proves that nothing else does: a host that holds only
# CRONOS_EMBED_KEY embeds a report, each of its users sees their own rows, the
# page runs under a strict Content-Security-Policy, and the key it holds cannot
# sign a portal session.
set -euo pipefail
cd "$(dirname "$0")/.."

export CRONOS_SIGNING_KEY="${CRONOS_SIGNING_KEY:-development-key-at-least-32-bytes-long}"
# The only key the host application is given.
export CRONOS_EMBED_KEY="${CRONOS_EMBED_KEY:-development-embed-key-32-bytes-long}"
export CRONOS_DEFINITIONS=demo/definitions
export CRONOS_SEED=demo/seed.sql
export CRONOS_ADDR="${CRONOS_ADDR:-:8789}"
export CRONOS_ORG=acme
export CRONOS_PROJECT=finance
HOST_PORT="${HOST_PORT:-5301}"
export CRONOS_URL="http://localhost:${CRONOS_ADDR#:}"
export HOST_URL="http://localhost:${HOST_PORT}"
export CRONOS_ORIGINS="$HOST_URL"

for port in "${CRONOS_ADDR#:}" "$HOST_PORT"; do
  if lsof -ti "tcp:${port}" >/dev/null 2>&1; then
    echo "freeing port ${port}"
    lsof -ti "tcp:${port}" | xargs kill 2>/dev/null || true
    sleep 1
  fi
done

# cronosd serves the element both sites load, so it is built in first.
./scripts/embed-bundle.sh
go build -o bin/cronosd ./cmd/cronosd

# The React site is a build; the plain one is the files as they are.
(cd packages/react && bun run build)
(cd integrations/react && VITE_CRONOS_URL="$CRONOS_URL" bun run build)

./bin/cronosd > /tmp/cronosd-integrations.log 2>&1 &
SERVER=$!
# The host's backend gets the embed key and the address, and not the signing key.
env -u CRONOS_SIGNING_KEY PORT="$HOST_PORT" bun integrations/server.ts > /tmp/cronos-host.log 2>&1 &
HOST=$!
trap 'kill $SERVER $HOST 2>/dev/null || true' EXIT

for _ in $(seq 1 40); do
  curl -sf -o /dev/null "${CRONOS_URL}/v1/health" &&
    curl -sf -o /dev/null "${HOST_URL}/api/session" && break
  sleep 0.25
done

bun packages/embed/scripts/integrations-check.mjs
