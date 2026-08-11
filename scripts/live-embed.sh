#!/usr/bin/env bash
# Drives packages/embed against a real cronosd, not a stub.
#
# The stub servers in the package checks assert that the component renders
# whatever it is handed. This asserts that what cronos actually sends is what
# the component can render — which is the integration nobody tests until it
# breaks in a customer's page.
set -euo pipefail
cd "$(dirname "$0")/.."

export CRONOS_SIGNING_KEY="${CRONOS_SIGNING_KEY:-development-key-at-least-32-bytes-long}"
export CRONOS_DEFINITIONS=demo/definitions
export CRONOS_SEED=demo/seed.sql
export CRONOS_ADDR="${CRONOS_ADDR:-:8788}"
PORT="${CRONOS_ADDR#:}"
export CRONOS_ORIGINS="http://localhost:${LIVE_PORT:-5199}"

go build -o bin/cronosd ./cmd/cronosd
go build -o bin/cronos-token ./cmd/cronos-token

./bin/cronosd > /tmp/cronosd-live.log 2>&1 &
SERVER=$!
trap 'kill $SERVER 2>/dev/null || true' EXIT

for _ in $(seq 1 40); do
  curl -sf -o /dev/null "http://localhost:${PORT}/v1/health" && break
  sleep 0.25
done

CRONOS_BASE="http://localhost:${PORT}" \
CRONOS_TOKEN="$(./bin/cronos-token -scope customer_id=c-1 -report billing-summary)" \
CRONOS_TOKEN_C2="$(./bin/cronos-token -scope customer_id=c-2 -report billing-summary)" \
  node packages/embed/scripts/live-check.mjs
