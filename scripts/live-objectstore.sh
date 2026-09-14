#!/usr/bin/env bash
#
# Reads a lake out of a real object store, with a key the store checks.
#
# Every other test of this path asserts the SQL a credential becomes. That says
# the statement is the intended one and nothing about whether a store accepts
# it, which is the only question a credential exists to answer — and the reason
# it went unasked is that there was no way to point cronos at anything but AWS.
# `endpoint:` is that way, and this is what proves it.
#
# What a unit test cannot answer, and this does:
#
#   1. a key written in a definition authenticates against a server
#   2. changing it stops the read — so the read was not an open bucket
#   3. a refusal does not carry the secret into the error, and so into a log
#   4. two lakes with a key each authenticate as themselves rather than as
#      whichever mounted last, which is what SCOPE on the secret is for
#
#   ./scripts/live-objectstore.sh
#
# Uses MinIO, which speaks S3. Set CRONOS_S3_ENDPOINT when something else
# manages the server and this will use it rather than starting one. Leaves
# nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

# Not 9000. That is the S3 port by convention, which is exactly why something
# else on a developer's machine is already listening on it — and a second
# server answering there returns AccessDenied for a bucket this never created,
# which reads as a credential fault rather than as the wrong server.
PORT=${CRONOS_S3_PORT:-19000}
CONTAINER=cronos-minio
ROOT_KEY=cronoskey
ROOT_SECRET=cronossecret123
work=$(mktemp -d)
started=""

RUNTIME=$(command -v podman || command -v docker || command -v container) || {
  echo "no container runtime — install podman or docker" >&2; exit 1; }
MC=(--rm --network="container:$CONTAINER" --entrypoint sh quay.io/minio/mc:latest -c)

cleanup() {
  rm -rf "$work"
  [ -n "$started" ] && "$RUNTIME" rm -f "$CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [ -z "${CRONOS_S3_ENDPOINT:-}" ]; then
  echo "==> starting MinIO on :$PORT"
  "$RUNTIME" rm -f "$CONTAINER" >/dev/null 2>&1 || true
  "$RUNTIME" run -d --name "$CONTAINER" -p "$PORT:9000" \
    -e MINIO_ROOT_USER="$ROOT_KEY" -e MINIO_ROOT_PASSWORD="$ROOT_SECRET" \
    quay.io/minio/minio:latest server /data >/dev/null
  started=1
  for _ in $(seq 1 60); do
    curl -sf "http://127.0.0.1:$PORT/minio/health/live" >/dev/null 2>&1 && break
    sleep 1
  done
  curl -sf "http://127.0.0.1:$PORT/minio/health/live" >/dev/null 2>&1 || {
    echo "MinIO never came up" >&2; exit 1; }
  export CRONOS_S3_ENDPOINT="http://127.0.0.1:$PORT"
fi

echo "==> writing two lakes"
mkdir -p "$work/events/day=1" "$work/events/day=2" "$work/other/day=1"
cat > "$work/mk.go" <<'EOG'
//go:build ignore
package main
import ("database/sql";"fmt";"os";_ "github.com/marcboeker/go-duckdb/v2")
func main() {
	db, _ := sql.Open("duckdb", "")
	d := os.Args[1]
	for _, q := range []string{
		`COPY (SELECT 'EU'::VARCHAR AS region, 100.0::DOUBLE AS amount) TO '` + d + `/events/day=1/p.parquet' (FORMAT PARQUET)`,
		`COPY (SELECT 'US'::VARCHAR AS region, 25.0::DOUBLE AS amount) TO '` + d + `/events/day=2/p.parquet' (FORMAT PARQUET)`,
		`COPY (SELECT 'APAC'::VARCHAR AS region, 7.0::DOUBLE AS amount) TO '` + d + `/other/day=1/p.parquet' (FORMAT PARQUET)`,
	} {
		if _, err := db.Exec(q); err != nil { fmt.Println(err); os.Exit(1) }
	}
}
EOG
go run -tags duckdb "$work/mk.go" "$work"

echo "==> two buckets, and a reader for each that cannot read the other"
"$RUNTIME" run -v "$work:/lake:ro" "${MC[@]}" '
set -e
mc alias set m http://127.0.0.1:9000 '"$ROOT_KEY $ROOT_SECRET"' >/dev/null
mc mb -p m/acme-lake m/lake-b >/dev/null
mc cp --recursive /lake/events m/acme-lake/ >/dev/null
mc cp --recursive /lake/other  m/lake-b/   >/dev/null
for b in acme-lake lake-b; do
  printf "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"s3:GetObject\",\"s3:ListBucket\"],\"Resource\":[\"arn:aws:s3:::%s\",\"arn:aws:s3:::%s/*\"]}]}" "$b" "$b" > /tmp/$b.json
done
mc admin user add m readera readera-secret-123 >/dev/null
mc admin user add m readerb readerb-secret-123 >/dev/null
mc admin policy create m only-a /tmp/acme-lake.json >/dev/null
mc admin policy create m only-b /tmp/lake-b.json    >/dev/null
mc admin policy attach m only-a --user readera >/dev/null
mc admin policy attach m only-b --user readerb >/dev/null' >/dev/null

echo "==> reading it as cronos would"
export CRONOS_S3_KEY=$ROOT_KEY CRONOS_S3_SECRET=$ROOT_SECRET
export CRONOS_S3_URI=s3://acme-lake/events/
export CRONOS_S3_KEY_A=readera CRONOS_S3_SECRET_A=readera-secret-123
export CRONOS_S3_KEY_B=readerb CRONOS_S3_SECRET_B=readerb-secret-123
export CRONOS_S3_URI_B=s3://lake-b/other/

go test -tags duckdb -count=1 -v -run 'LiveObjectStore|TwoLiveLakes' \
  ./internal/adapter/driver/duckdb/

echo
echo "ok  a key in a definition reaches a real store, and a wrong one does not"
