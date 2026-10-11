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
# No CRONOS_EMBED_KEY here, on purpose: this is a deployment that has not set
# one, where embed tokens are still signed with the signing key. The separate
# key is driven by scripts/live-integrations.sh, so both arrangements are run.
unset CRONOS_EMBED_KEY CRONOS_EMBED_KEY_PREVIOUS
export CRONOS_DEFINITIONS=demo/definitions
export CRONOS_SEED=demo/seed.sql
export CRONOS_ADDR="${CRONOS_ADDR:-:8788}"
# The project this server serves, and so the one cronos-token mints for: it
# reads the same two variables. The embed handler reads the organisation and
# project out of a token, so a token naming another project opens nothing here.
export CRONOS_ORG=acme
export CRONOS_PROJECT=finance
PORT="${CRONOS_ADDR#:}"
export CRONOS_ORIGINS="http://localhost:${LIVE_PORT:-5199}"

# Free the page port first. A previous run that failed an assertion used to
# leave its server listening, and the next one then hung against a stale page
# rather than saying what was wrong.
PAGE_PORT="${LIVE_PORT:-5199}"
if lsof -ti "tcp:${PAGE_PORT}" >/dev/null 2>&1; then
  echo "freeing port ${PAGE_PORT}"
  lsof -ti "tcp:${PAGE_PORT}" | xargs kill 2>/dev/null || true
  sleep 1
fi

# The element the page loads comes from cronosd itself, so it is built in.
./scripts/embed-bundle.sh
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
  bun packages/embed/scripts/live-check.mjs
