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
| `CRONOS_SMTP_HOST`, `CRONOS_SMTP_FROM` | A mail relay. Needed to deliver a schedule by email, and to invite anybody. |
| `CRONOS_PORTAL_URL` | Where the portal is served, for links in email. Without it, invitations are not offered — there would be nothing to put in the link. |

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

## Adding people

Two ways, and the first is the one to use.

**An invitation.** `POST /v1/people` with an email, a name and a role and *no*
password. Nothing is created: a single-use secret is mailed to them, and the
account comes into being when they set a password only they have ever seen.
Needs `CRONOS_SMTP_HOST`, `CRONOS_SMTP_FROM` and `CRONOS_PORTAL_URL`; the portal
asks whether they are set and offers the choice only where they are.

**A password you choose.** The same endpoint with a password. The account exists
immediately and somebody other than its owner knows its credential — which then
lives in whatever carried it, a chat message or a ticket or a sent folder, known
to at least two people from the moment it exists. Kept because a deployment
without a mail relay has to be able to add its second administrator somehow, and
because the first one is created by `cronos-user` the same way.

The invitation's secret is 256 bits from `crypto/rand`, stored as a SHA-256 hash
and never in the clear, spent by the `UPDATE` that accepts it, and dead after a
week. It travels in the link's **fragment** — after the `#` — which a browser
sends to no server, so it is not in the portal's access log, its CDN's, or a
`Referer` header. `GET /v1/auth/invitation` answers the same for expired, spent
and never-issued, because that endpoint has no session by design and telling
them apart would say which addresses somebody is onboarding.

Rows are swept hourly: unusable ones as soon as they expire, accepted ones after
thirty days, by which point the audit log is the record and what is left here is
an address kept by accident.

```bash
./scripts/live-invite.sh
```

Runs a real SMTP server and a real database, invites somebody, reads the message
out of the mailbox, follows the link, and then checks the two things that matter
most: that the link does not work twice, and that replaying it changes nothing.

## Ending sessions

A portal token is signed and stateless: nothing is written when one is minted,
so there is no list of sessions to show and no way to end one and keep another.
The account page used to offer "sign out everywhere else" over a list of devices
and cities that were invented in the browser — a control that did nothing beside
data that was not true.

What replaced it is one timestamp per account, in `cronos_sessions_cut`. Every
request already asks whether the account is still active; it now also asks when
its sessions were cut, and refuses any token minted before that. `POST
/v1/auth/sessions/end` draws the line and hands the calling browser a fresh
token dated at it — so the person who pressed the button stays signed in and
every other machine is signed out.

The line falls on the **next second boundary**, not on the current instant. A
token's `iat` has second granularity, so a line drawn part-way through a second
cannot be told from a session minted earlier in that same second — driving it
with two real browsers found exactly that, and rounding up removes the ambiguity
instead of narrowing it. The replacement token's `iat` is therefore up to a
second in the future, well inside the 30 seconds of skew every token is already
verified with.

It takes effect within about five seconds, which is how long the standing answer
is cached — the same trade already made for disabling somebody.

```bash
./scripts/live-sessions.sh
```

## Two-factor authentication

TOTP, RFC 6238, against any authenticator app. Enabled per account by its owner;
there is deliberately no path by which an administrator enrols, inspects or
removes somebody else's — enrolling for another person is meaningless, they hold
the phone, and removing one is exactly what a social-engineering call asks for.

The portal has shown an enrolment wizard since before there was a server to
enrol against. It accepted any six digits, the QR code beside it was noise with
finder squares drawn in the corners, and the recovery codes were generated in
the browser and stored nowhere. An account could finish that wizard and be
marked as protected by a secret that existed in no app anywhere — which is worse
than offering nothing, because its owner then picks a weaker password.

What is enforced now:

- **Enrolment is proved.** Nothing is switched on until a code computed from the
  stored secret comes back. `confirmed_at` is NULL until then and `Protected`
  reads it, so an abandoned enrolment protects nothing and is replaced by the
  next attempt.
- **A code is spent.** It is valid for a whole thirty-second step, long enough to
  be read off a shoulder or a screenshot. The step is recorded, and the check is
  inside the `UPDATE` — two sign-ins racing with one stolen code let one in.
- **The secret is write-once.** Readable while enrolment is in progress, so a
  reloaded page can show the QR again, and never afterwards. An endpoint that
  hands it back turns a stolen session into a permanent second factor of the
  attacker's own.
- **Turning it off takes a code**, or a recovery code. Without that, a stolen
  session strips the factor off the account it stole at the moment the factor is
  all that is left.
- **Recovery codes are passwords.** Ten, from Crockford's alphabet so nothing is
  misread, shown once at confirmation, stored as SHA-256 hashes, spent by the
  `DELETE` that checks them, and deleted with the factor.

Sign-in sends the password and the code together. A two-step exchange would need
a challenge that says "this password was right", and that challenge is a
credential to steal, expire, rate limit, and — worst — a way to learn which
accounts have a factor by watching which ones issue one. Instead, an attempt
with the password alone against a protected account answers `{"factorRequired":
true}`, which is only ever said to somebody who already proved the password.

The two steps have **separate rate limits**, which a live run forced: they shared
one, so three mistyped codes locked somebody out of their own password. The
password limit stays tight because password lists are long; the code limit is
looser because the legitimate retry is common and guessing six digits is
hopeless at any rate a person would tolerate.

```bash
./scripts/live-2fa.sh
```

Enrols against a running server and computes every code independently, in
Python, from the `otpauth://` URI — because a test that checks cronos against
cronos passes just as well when both halves are wrong, which is how the old
wizard shipped. It takes about forty seconds: most of that is waiting for the
sign-in rate limit to refill, which is the limiter working.

## Checking the portal itself

```bash
./scripts/live-portal-2fa.sh
```

Starts a server on a fresh database, starts the portal against it, and drives
two-factor enrolment through a real browser — computing every code in Node
rather than asking cronos what to type.

It exists because the type checker, the linter and the API checks all passed
while the enrolment wizard was rendering *inside* the account page, between the
password and the sessions, with two sets of Back/Continue on screen; and while
two sibling panels were keyed by the same counter, so React was quietly
discarding one. Neither is a thing any of those tools can see.

## The first run

A fresh install has a database, a server and no accounts. Until there was a
`/setup`, the only way to create the first one was `cronos-user` on the machine —
fine for somebody with shell access, impossible for anybody handed a URL.

Open the portal. It offers to set the deployment up, takes an email, a password
and the names of the first organisation and project, and signs you in as an
account that administers both the deployment and that project.

It is the most dangerous endpoint in the product, so:

- It is open only while **no account exists at all** — not "no administrator",
  no account, anywhere. The first success closes it and nothing reopens it short
  of emptying the users table.
- The check happens inside the write, and the write is one transaction in the
  database — a marker row with a fixed key, inserted alongside the account and
  the grant. Two people sent the same URL create one administrator; so do two
  cronos processes brought up against one empty database before anybody has the
  address, which a mutex in one process could not have managed. A losing request
  writes nothing at all, so the address it used is not consumed.
- An upgrade is not a first run. Deployments that predate the marker row have
  accounts and no marker, so the check counts them too — without that, every
  upgrade would offer its next visitor a deployment administrator.
- It needs a store. A file-backed deployment has nowhere to keep an account, so
  the page is not offered rather than offered and broken.
- There is no token in the log and no environment variable to unlock it. Both
  are things people leave switched on.

The names you type become identifiers — "Acme Logistics" is stored as
`acme-logistics` — because they are half of every tenancy check and part of a
path when definitions live on disk. A single-project process adopts the name it
is given at that moment; one told what to serve by `CRONOS_PROJECTS` keeps what
it was told, and says so in the log if a first run names something else.

```bash
./scripts/live-setup.sh
```

## Administering the deployment

A tier above organisations, for whoever runs the servers. **Settings →
Deployment**, visible only to an account that holds it.

It can list every tenant and every account, **create the first account of an
organisation that has none**, move somebody from one organisation to another,
turn access off anywhere, and grant or revoke itself.

Creating one names the organisation and project, which is the single thing no
project administrator may do — `/v1/people` always creates into the caller's own
project. The password is chosen rather than emailed: an invitation is addressed
to a project's own people by somebody who works with them, and this is a
deployment operator standing up a customer who has no account, no project and
nobody to invite them yet.

It **cannot read any project's data**. Opening a report, running a query or
seeing a dataset still requires membership in that project — `Principal.Platform`
is deliberately absent from `CanRead`, `CanEdit`, `CanAdminProject` and
`CanAdminOrg`, and there is no route under `/v1/platform` that returns a row of
anybody's warehouse. That is what makes a leaked platform credential a
control-plane problem rather than every customer's data at once. Support that
needs to see what a customer sees adds itself to that project, and the audit log
records that it did.

Somebody without the permission is answered **404**, not 403: a "you may not"
would confirm to anybody probing that the tier exists and that some account
holds it.

The last administrator cannot be revoked. A deployment with none cannot make
another over HTTP, because the endpoints that grant it require the permission
being granted. The way back is the command line:

```bash
cronos-user -dsn "$CRONOS_STORE_DSN" -email ops@acme.example -platform
```

It grants the permission to an account that already exists and asks for no
password: the account being rescued is somebody else's, and a command that reset
their password in order to grant a permission would lock its owner out to let
them back in. On a new account, `-platform` alongside the usual flags creates
and grants in one go. There is deliberately no `-revoke`: removing it is the
API's job, where the guard against taking the last one lives. Revoking anybody
else ends their sessions in the same transaction, because the permission travels
in the token and would otherwise outlive the revocation by up to eight hours.

Every cross-tenant action is audited under its own `platform.*` prefix, so "who
reached across tenants, and when" is a question the log answers directly.

## Requiring a second factor of everyone

**Settings → Security**, per project, by a project administrator.

The flag was never the hard part. What made this wait is that somebody with no
second factor cannot enrol without signing in and cannot sign in without
enrolling — so refusing the sign-in locks a team out of its own reporting on the
afternoon it is switched on, and puts an administrator on the phone being asked
to turn a second factor off. That is the exact call a second factor exists to
make suspicious.

So nobody is refused. Somebody without one signs in normally and gets a session
that reaches the enrolment routes and **nothing else** — every other route
answers 403, and the portal shows the wizard instead of a shell of refusals.
Finishing it hands back an ordinary session in the same response, so they are not
asked to sign in again thirty seconds after proving a password and a code.

The gate is an allow-list wrapped around the whole API rather than a check per
handler, and the direction matters: a route added tomorrow is refused to these
sessions until somebody deliberately lists it. A deny-list would let every new
route through and nobody would be looking.

Turning it on does not touch sessions that already exist, including the one that
turned it on — the requirement bites at the next sign-in. The panel shows how
many people have one before you switch it, because "12 people, 4 without" is the
whole decision.

```bash
./scripts/live-require-2fa.sh
```

## The live checks

`scripts/live-*.sh` drive a running cronos the way a person or a browser would.
Between them they have found around a dozen defects no unit test could: a QR code
that encoded nothing, a session cut that spared the phone it was meant to end, a
chart that compiled to `GROUP BY 1` against a database that reads the `1` as a
constant, an enrolment wizard rendering inside the account page.

| Script | What it drives | Needs |
|---|---|---|
| `live-setup.sh` | first run, platform administration, the CLI recovery path | go |
| `live-sessions.sh` | ending every other session | go |
| `live-2fa.sh` | enrolment and sign-in, with codes computed in Python | go, python3 |
| `live-require-2fa.sh` | turning the requirement on, and the enrolment-only session | go, python3 |
| `live-invite.sh` | inviting somebody and reading the mail | a mail server |
| `live-sso.sh` | a whole OIDC sign-in and single log-out | Keycloak |
| `live-sqlserver.sh` | a report against SQL Server | SQL Server |
| `live-portal-2fa.sh` | the same enrolment, through a browser | bun, chromium |

The first five run in CI on every push, in about a minute; SQL Server is its own
job because the image is 1.7 GB and nothing else should wait for it. The two that
want a Keycloak or a browser are still run by hand.

A script that finds a server already listening uses it and leaves it alone, so
CI's service containers work without the script knowing it is CI. The exception
is `live-sqlserver.sh`, which needs telling: set `CRONOS_SQLSERVER_RUNNING=1`.
Locally it starts Azure SQL Edge instead, because `mssql/server:2022` is amd64
and segfaults on an arm64 laptop under emulation — the same engine either way.
