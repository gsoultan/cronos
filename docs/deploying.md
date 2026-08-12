# Deploying cronos

One process serves one project. `CRONOS_ORG` and `CRONOS_PROJECT` pin it, and
every token it accepts is checked against them. The definition store is
multi-tenant — every statement is scoped by organization and project taken from
the caller's identity — but the process is not, so a deployment serving several
projects runs several processes against one database. That is a deliberate
shape: the blast radius of a bad definition, a runaway query or a leaked
signing key is one customer's project.

## The image

```bash
make image                     # builds, and proves the typesetter is in it
docker build -t cronos:1.0 .
```

Three stages: the portal's static files, the server binaries, and `typst`.
That last one is the only thing an image can be missing without saying so —
every path except paginated output works without it, so a PDF schedule fails
at six in the morning on the first of the month and nowhere earlier. Both
`make image` and CI run `typst --version` inside the result for exactly that
reason.

The image is CGO-free and runs as an unprivileged user. A deployment that
federates across databases needs `-tags duckdb`, which needs cgo, and builds
its own image from a base with the runtime.

`TZ` is set and `tzdata` is installed, because "the first of the month at six"
is a local claim. A container with no zoneinfo resolves every timezone to UTC,
which is a statement dated an hour early in the wrong month.

## What has to be set

| Variable | Why |
|---|---|
| `CRONOS_SIGNING_KEY` | 32 bytes minimum. Every token is signed with it; rotating it ends every session and every share. |
| `CRONOS_ORG`, `CRONOS_PROJECT` | The one project this process serves. |
| `CRONOS_STORE_DSN` | The definition store. Without it, definitions are files and there is no sign-in, no run history and no sharing. |
| `CRONOS_ORIGINS` | The host applications allowed to call the API. Never `*` — the API reads an Authorization header. |

And what should be:

| Variable | Why |
|---|---|
| `CRONOS_SECRETS_DIR` | A directory of files, one per secret. Preferred over the environment: an environment variable is visible in `/proc` to anything running as the same user and appears in a crash dump. |
| `CRONOS_HISTORY_RETENTION` | e.g. `2160h`. Unset keeps run history for ever. |
| `CRONOS_BEHIND_PROXY=1` | Only where something in front sets `X-Forwarded-For`. Believing it without a proxy keys every rate limit by a value the caller chooses. |
| `CRONOS_AUDIT` | `log` by default, `off` to stop it. |

## Probes

- `/v1/health` — liveness. Unconditional: a liveness probe that fails because
  a database is unreachable restarts a healthy process and does not fix the
  database.
- `/v1/ready` — readiness, which asks. 503 when the store is unreachable or at
  a schema this build does not know; 200 and `degraded` when a datasource is
  down, because the reports that do not read it still work.
- `/v1/metrics` — Prometheus exposition.

The alerts worth having, in order of how much they cost when missed:

1. `cronos_deliveries_total{result="failed"}` rising. Somebody's invoice did
   not arrive, and nothing else will tell you.
2. `cronos_runs_total{result="failed"}` — a schedule that did not run at all.
3. `/v1/ready` returning `degraded` for longer than a deploy takes.
4. `cronos_request_duration_seconds` at the 99th percentile crossing whatever
   your customers' patience is.

## Upgrading

Migrations are forward-only and run at startup, each in a transaction. A
database at a schema newer than the binary is refused rather than written to,
so a rollback needs the old data and a restore, not a downgrade.

Roll one instance at a time. Two versions against one database is ordinary
during a deploy; what is not is an old binary writing to a schema a new one has
already changed, which is what the refusal prevents.

## Backing up

Two things hold state, and they are not equally replaceable.

**The definition store.** Definitions, their version history, users, shares and
run history. Back it up the way you back up any Postgres — this schema has no
special requirement, and every table carries the tenant it belongs to. Restore
is a restore.

**The signing key.** Not in the database, and losing it is not recoverable:
every session ends, every share link stops opening, and every embed token a
host application holds becomes invalid. Keep it wherever you keep the thing
that would end your business if you lost it, and rotate it deliberately — a
rotation is an outage for every embedded reader until the hosts mint new
tokens.

The definitions directory is not state. It is a bootstrap: once the store holds
anything, the directory is not consulted again.

## Restoring, and proving you can

A backup nobody has restored is a hypothesis. The drill, in full:

```bash
# 1. Restore into an empty database.
psql "$RESTORE_DSN" < backup.sql

# 2. Point a cronosd at it with the real signing key, on a port nobody uses.
CRONOS_STORE_DSN="$RESTORE_DSN" CRONOS_ADDR=:9999 cronosd

# 3. It must reach ready, which proves the schema is the one this build knows.
curl -sf localhost:9999/v1/ready

# 4. And a report must render, which proves the definitions came back whole.
curl -sf -X POST localhost:9999/v1/reports/<a-real-report> \
  -H "authorization: Bearer $(cronos-token -audience portal -role viewer …)" \
  -d '{}'
```

Step 4 is the one that matters. A database that restores and a product that
serves are different claims, and only the second is the one anybody is asking
about.
