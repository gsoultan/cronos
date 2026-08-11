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
PORT="${CRONOS_ADDR#:}"
WEB_PORT="${PORTAL_PORT:-5174}"
export CRONOS_ORIGINS="http://localhost:${WEB_PORT}"

for p in "$PORT" "$WEB_PORT"; do
  if lsof -ti "tcp:${p}" >/dev/null 2>&1; then
    lsof -ti "tcp:${p}" | xargs kill 2>/dev/null || true
    sleep 1
  fi
done

go build -o bin/cronosd ./cmd/cronosd
go build -o bin/cronos-token ./cmd/cronos-token

./bin/cronosd > /tmp/cronosd-portal.log 2>&1 &
SERVER=$!
trap 'kill $SERVER 2>/dev/null || true; kill $WEB 2>/dev/null || true' EXIT

for _ in $(seq 1 40); do
  curl -sf -o /dev/null "http://localhost:${PORT}/v1/health" && break
  sleep 0.25
done

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

BASE="http://localhost:${WEB_PORT}" node apps/portal/scripts/live-portal-check.mjs
