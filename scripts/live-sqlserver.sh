#!/usr/bin/env bash
#
# Runs a report against a real SQL Server.
#
# The dialect is thirty lines and its unit tests check the strings it produces.
# What they cannot check is whether SQL Server agrees: whether `@p1` binds where
# the compiler thinks it does, whether the truncation arithmetic buckets the
# months a person would expect, and whether the driver hands back types the
# renderer can read. Every one of those is a boundary with the database on the
# other side.
#
#   ./scripts/live-sqlserver.sh
#
# Uses Azure SQL Edge, which is the SQL Server engine and the only one of the
# family that runs on arm64 — mssql/server:2022 is amd64 and segfaults under
# emulation. The T-SQL below is the subset cronos generates, so agreement here
# is agreement on a real server.
set -euo pipefail

cd "$(dirname "$0")/.."

PORT=8801
API="http://localhost:$PORT"
SA_PASSWORD='A-Strong-Passw0rd!'

work=$(mktemp -d)
cleanup() {
	[ -n "${srv:-}" ] && { kill "$srv" 2>/dev/null || true; }
	[ -n "${started_db:-}" ] && podman rm -f cronos-mssql >/dev/null 2>&1
	rm -rf "$work" || true
	return 0
}
trap 'cleanup 2>/dev/null' EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok() { printf '  \033[32mok\033[0m %s\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }

# SQL is run through go-mssqldb — the same driver cronos opens the source with —
# rather than through sqlcmd, which the Azure SQL Edge image does not ship. That
# is the better tool for the job anyway: the fixture and the dialect queries then
# travel the same path a report does, so a driver that mangled a type would be
# caught while setting up rather than blamed on the renderer.
sql() { go run "$work/sql.go" "$1"; }

# `grep -q` on a pipe is a trap under `set -o pipefail`.
#
# grep exits the moment it matches, the writer gets SIGPIPE, and the pipeline's
# status is that failure — so a successful search reads as a failed one, and the
# script dies saying the container never started when it started fine. Reading
# the whole thing into a variable first costs nothing at this size and cannot
# lie.
logged() { podman logs cronos-mssql 2>&1 || true; }
ready() { case "$(logged)" in *"ready for client connections"*) return 0 ;; esac; return 1; }

# Asking the server itself, for when there is no container to read a log from.
# It is also the better question: a log line says the process started, and this
# says it will answer a query.
answers() { [ -n "$(sql 'SELECT 1' 2>/dev/null || true)" ]; }

cat >"$work/sql.go" <<'GO'
//go:build ignore

// A two-minute psql for SQL Server, using the driver under test.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/microsoft/go-mssqldb"
)

func main() {
	db, err := sql.Open("sqlserver", os.Getenv("DSN"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()

	// Statements one at a time: SQL Server refuses USE and DDL in one batch
	// through this driver, and splitting here keeps the fixtures readable.
	for _, stmt := range strings.Split(os.Args[1], ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		rows, err := db.Query(stmt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n  %v\n", strings.TrimSpace(stmt), err)
			os.Exit(1)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			cells := make([]any, len(cols))
			into := make([]any, len(cols))
			for i := range cells {
				into[i] = &cells[i]
			}
			if err := rows.Scan(into...); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			out := make([]string, len(cells))
			for i, c := range cells {
				// DECIMAL comes back as []byte from this driver, which prints
				// as a list of character codes unless it is turned back into
				// the text it is. Worth knowing: whatever reads these rows for
				// real has the same job.
				if b, ok := c.([]byte); ok {
					out[i] = string(b)
					continue
				}
				out[i] = fmt.Sprintf("%v", c)
			}
			fmt.Println(strings.Join(out, "\t"))
		}
		rows.Close()
	}
}
GO

export DSN="sqlserver://sa:$SA_PASSWORD@localhost:1433?encrypt=disable"

say "SQL Server"

# Somebody else's server, or one of ours.
#
# CI supplies it as a service container — mcr.microsoft.com/mssql/server, which
# is the real thing and runs on the amd64 runners. Locally that image segfaults
# on arm64 under emulation, so this starts Azure SQL Edge instead: the same
# engine, and the only member of the family that runs on an Apple laptop.
#
# Set CRONOS_SQLSERVER_RUNNING when something else is managing it. Then this
# waits on the port rather than on a container's log, because there is no
# container here to read the log of.
if [ -n "${CRONOS_SQLSERVER_RUNNING:-}" ]; then
	for _ in $(seq 1 60); do
		answers && break
		sleep 5
	done
	answers || die "nothing is answering on 1433"
else
	if [ -z "$(podman ps --filter name=cronos-mssql --format '{{.Names}}' 2>/dev/null || true)" ]; then
		started_db=yes
		podman rm -f cronos-mssql >/dev/null 2>&1 || true
		podman run -d --name cronos-mssql -p 1433:1433 \
			-e ACCEPT_EULA=1 -e "MSSQL_SA_PASSWORD=$SA_PASSWORD" \
			mcr.microsoft.com/azure-sql-edge:latest >/dev/null
	fi
	for _ in $(seq 1 60); do
		ready && break
		sleep 5
	done
	ready || die "never came up"
fi


case "$(sql "SELECT @@VERSION")" in *SQL*) ;; *) die "cannot query it" ;; esac
ok "running and answering"

# --- the data ------------------------------------------------------------------

say "A warehouse to read"

sql "IF DB_ID('erp') IS NULL CREATE DATABASE erp" >/dev/null
DSN="sqlserver://sa:$SA_PASSWORD@localhost:1433?database=erp&encrypt=disable"
sql "
IF OBJECT_ID('dbo.invoices') IS NOT NULL DROP TABLE dbo.invoices;
CREATE TABLE dbo.invoices (
  id INT PRIMARY KEY, customer NVARCHAR(80) NOT NULL,
  placed_at DATETIME2 NOT NULL, amount DECIMAL(12,2) NOT NULL);
INSERT INTO dbo.invoices VALUES
  (1,'Acme','2026-01-05T09:00:00',100.00),
  (2,'Acme','2026-01-20T09:00:00',150.00),
  (3,'Acme','2026-02-11T09:00:00',300.00),
  (4,'Globex','2026-02-14T09:00:00',75.50),
  (5,'Globex','2026-03-02T09:00:00',20.00);
" >/dev/null
case "$(sql "SELECT COUNT(*) FROM dbo.invoices")" in *5*) ;; *) die "the fixture is not there" ;; esac
ok "erp.dbo.invoices, five rows across three months"

# --- what the dialect produces --------------------------------------------------

say "The SQL cronos generates, run by the server"

# The monthly bucket, exactly as internal/core/query/sqlserver.go writes it.
months=$(sql "SELECT CONVERT(varchar(10), DATEADD(month, DATEDIFF(month, 0, placed_at), 0), 23) AS m,
       SUM(amount) AS total
FROM dbo.invoices GROUP BY DATEADD(month, DATEDIFF(month, 0, placed_at), 0)
ORDER BY 1;")

for month in 2026-01-01 2026-02-01 2026-03-01; do
	case "$months" in *"$month"*) ;; *) printf '%s\n' "$months"; die "$month is not a bucket" ;; esac
done
ok "DATEADD/DATEDIFF buckets to the first of each month"

# And the totals are the ones a person would expect, which is the check that
# the arithmetic rounds down rather than to nearest.
case "$months" in *250*) ;; *) printf '%s\n' "$months"; die "January's total is wrong" ;; esac
case "$months" in *375*) ;; *) printf '%s\n' "$months"; die "February's total is wrong" ;; esac
ok "and the months add up"

# Quarter and year, the other two grains this dialect offers.
q=$(sql "SELECT CONVERT(varchar(10), DATEADD(quarter, DATEDIFF(quarter, 0, placed_at), 0), 23) FROM dbo.invoices WHERE id = 3;")
case "$q" in *2026-01-01*) ;; *) printf '%s\n' "$q"; die "February is not in Q1" ;; esac
y=$(sql "SELECT CONVERT(varchar(10), DATEADD(year, DATEDIFF(year, 0, placed_at), 0), 23) FROM dbo.invoices WHERE id = 5;")
case "$y" in *2026-01-01*) ;; *) die "March is not in 2026" ;; esac
ok "quarter and year too"

# DATETRUNC is what this dialect deliberately does not use. Whether this engine
# has it is not the point — the point is that a 2016 server does not, and the
# form above works on both.
ok "and none of it needs DATETRUNC, which is 2022 and later"

# --- through cronos -------------------------------------------------------------

say "Through cronos"
go build -o bin/cronosd ./cmd/cronosd || die "build"

mkdir -p "$work/defs"
cat >"$work/defs/source.yaml" <<YAML
apiVersion: cronos.dev/v1
kind: DataSource
metadata:
  name: erp
spec:
  driver: sqlserver
  dsn: "sqlserver://sa:$SA_PASSWORD@localhost:1433?database=erp&encrypt=disable"
YAML

cat >"$work/defs/dataset.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Dataset
metadata:
  name: invoices
spec:
  sources:
    - ref: erp

  # The name is qualified because SQL Server puts everything in a schema, and
  # `invoices` alone resolves against whatever the login's default is — which is
  # dbo here and is not everywhere.
  query: |
    SELECT customer, placed_at, amount FROM dbo.invoices

  fields:
    - {name: customer,  type: string, role: dimension}
    - {name: placed_at, type: date,   role: dimension}
    - {name: amount,    type: number, role: measure, aggregate: sum}
YAML

cat >"$work/defs/report.yaml" <<'YAML'
apiVersion: cronos.dev/v1
kind: Report
metadata:
  name: monthly
  title: Monthly invoices
spec:
  dataset: invoices
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: chart
          chart: bar
          title: Billed by month
          x: {field: placed_at, grain: month}
          y: {field: amount, aggregate: sum}
YAML

CRONOS_ADDR=":$PORT" CRONOS_DEFINITIONS="$work/defs" \
	CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
	CRONOS_ADMIN_KEY=admin-key-for-a-local-test \
	./bin/cronosd >"$work/log" 2>&1 &
srv=$!
for _ in $(seq 1 40); do
	curl -sf "$API/v1/health" >/dev/null 2>&1 && break
	sleep 0.25
done
curl -sf "$API/v1/health" >/dev/null || { cat "$work/log"; die "cronos never came up"; }
ok "opened the source and loaded the definitions"

probe=$(curl -s -X POST -H "Authorization: Bearer admin-key-for-a-local-test" \
	"$API/v1/datasources/erp/test")
case "$probe" in *'"ok":true'*) ;; *) printf '  %s\n' "$probe"; tail -5 "$work/log"; die "the connection test failed" ;; esac
ok "the connection test passes"

report=$(curl -s -H "Authorization: Bearer admin-key-for-a-local-test" "$API/v1/reports/monthly")
case "$report" in *'Jan 2026'*) ;; *) printf '  %s\n' "$(printf '%s' "$report" | head -c 400)"; tail -5 "$work/log"; die "the report has no January" ;; esac
case "$report" in *'"value":250'*) ;; *) die "January's total is not 250" ;; esac

# The decimal survived the round trip. This driver hands DECIMAL back as bytes
# rather than a number, so a renderer that took the value at face value would
# print a list of character codes — worth asserting rather than assuming, since
# it is the one type SQL Server returns differently from the others.
case "$report" in *'"value":375.5'*) ;; *) printf '  %s\n' "$report"; die "February's total is not 375.50" ;; esac
case "$report" in *'"formatted":"375.50"'*) ;; *) die "the decimal was not formatted" ;; esac
ok "a report grouped by month comes back with the right numbers"
ok "and a DECIMAL survives as a number rather than as bytes"

say "All of it worked."
