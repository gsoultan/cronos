#!/usr/bin/env bash
#
# Reads a lake out of Azure Blob, with a key the service checks.
#
# The companion to live-objectstore.sh, and not a copy of it. Azure's credential
# is not a key beside a secret: its secret takes a connection string, and this
# build assembles one out of the account and key a definition names. An
# assembled string is exactly what looks right in a unit test and is refused by
# the service, so only the service can answer.
#
# What a unit test cannot answer, and this does:
#
#   1. the connection string cronos builds is one Azure accepts
#   2. changing the key stops the read — so the read was not an open container
#   3. a refusal does not carry the account key into the error, and so into a log
#
#   ./scripts/live-azure.sh
#
# Uses Azurite, Microsoft's own emulator. Set CRONOS_AZURE_ENDPOINT when
# something else manages the server and this will use it rather than starting
# one. Leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

# Not 10000. That is Azurite's default and therefore the port another copy of it
# is already on, and a second emulator answering there is a credential fault
# that is really a wrong server — the same trap live-objectstore.sh documents.
PORT=${CRONOS_AZURE_PORT:-11000}
CONTAINER=cronos-azurite
# Azurite's well-known development account. Public, documented by Microsoft, and
# not a secret — which is why it can be written here and a real one cannot.
ACCOUNT=devstoreaccount1
KEY='Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=='
work=$(mktemp -d)
started=""

RUNTIME=$(command -v podman || command -v docker || command -v container) || {
  echo "no container runtime — install podman or docker" >&2; exit 1; }

cleanup() {
  rm -rf "$work"
  [ -n "$started" ] && "$RUNTIME" rm -f "$CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [ -z "${CRONOS_AZURE_ENDPOINT:-}" ]; then
  echo "==> starting Azurite on :$PORT"
  "$RUNTIME" rm -f "$CONTAINER" >/dev/null 2>&1 || true
  "$RUNTIME" run -d --name "$CONTAINER" -p "$PORT:10000" \
    mcr.microsoft.com/azure-storage/azurite:latest \
    azurite-blob --blobHost 0.0.0.0 --blobPort 10000 --skipApiVersionCheck >/dev/null
  started=1
  for _ in $(seq 1 60); do
    curl -s -o /dev/null -m 2 "http://127.0.0.1:$PORT/$ACCOUNT?comp=list" && break
    sleep 1
  done
  export CRONOS_AZURE_ENDPOINT="http://127.0.0.1:$PORT"
fi

echo "==> writing parquet"
cat > "$work/mk.go" <<'GO'
//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/marcboeker/go-duckdb/v2"
)

func main() {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, q := range []string{
		`COPY (SELECT 'EU'::VARCHAR AS region, 100.0::DOUBLE AS amount) TO '` + os.Args[1] + `/day=1/p.parquet' (FORMAT PARQUET)`,
		`COPY (SELECT 'US'::VARCHAR AS region, 25.0::DOUBLE AS amount) TO '` + os.Args[1] + `/day=2/p.parquet' (FORMAT PARQUET)`,
	} {
		if _, err := db.Exec(q); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
GO
mkdir -p "$work/events/day=1" "$work/events/day=2"
go run -tags duckdb "$work/mk.go" "$work/events"

echo "==> uploading"
# The SDK rather than a signed request by hand. Azure's SharedKey signature is
# a dozen fields in a fixed order and getting one wrong returns the same 403 as
# a wrong key — which is the exact confusion this script exists to rule out.
VENV=${CRONOS_AZURE_VENV:-/tmp/cronos-azvenv}
[ -x "$VENV/bin/python" ] || python3 -m venv "$VENV" >/dev/null
"$VENV/bin/pip" install --quiet azure-storage-blob >/dev/null
CONN="DefaultEndpointsProtocol=http;AccountName=$ACCOUNT;AccountKey=$KEY;BlobEndpoint=$CRONOS_AZURE_ENDPOINT/$ACCOUNT;"
CONN="$CONN" WORK="$work" "$VENV/bin/python" - <<'PY'
import os
from azure.storage.blob import BlobServiceClient

svc = BlobServiceClient.from_connection_string(os.environ["CONN"])
try:
    svc.create_container("lake")
except Exception:
    pass
for day in ("day=1", "day=2"):
    path = f"{os.environ['WORK']}/events/{day}/p.parquet"
    with open(path, "rb") as f:
        svc.get_blob_client("lake", f"events/{day}/p.parquet").upload_blob(f, overwrite=True)
PY

export CRONOS_AZURE_ACCOUNT=$ACCOUNT CRONOS_AZURE_KEY=$KEY
export CRONOS_AZURE_URI=az://lake/events/

echo "==> reading it back through cronos"
go test -tags duckdb -count=1 -v -run 'LiveAzure' ./internal/adapter/driver/duckdb/

echo
echo "ok  an account and key in a definition reach Azure, and a wrong key does not"
