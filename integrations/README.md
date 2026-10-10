# Example host applications

A cronos report inside somebody else's website, twice: once with plain
JavaScript and once with React. The same backend serves both, because the
backend is the same either way — it is the front end that differs.

```
integrations/
  token.ts      signing an embed token: the thirty lines a host backend needs
  server.ts     the host's backend: who is signed in, which rows they may read, the token route
  plain/        HTML and a script. No framework, no build step
  react/        a Vite + React application, using @cronos/react
```

## Run them

Everything, checked in a real browser:

```bash
./scripts/live-integrations.sh
```

Or by hand, to look at them:

```bash
# 1. cronos, with the demo reports, allowing this host's origin
export CRONOS_SIGNING_KEY="development-key-at-least-32-bytes-long"
export CRONOS_EMBED_KEY="development-embed-key-32-bytes-long"
export CRONOS_ORG=acme CRONOS_PROJECT=finance
export CRONOS_ORIGINS="http://localhost:5301"
./scripts/embed-bundle.sh       # so cronosd serves the element
CRONOS_DEFINITIONS=demo/definitions CRONOS_SEED=demo/seed.sql go run ./cmd/cronosd

# 2. the React site is a build (the plain one is the files as they are)
(cd packages/react && bun run build)
(cd integrations/react && VITE_CRONOS_URL=http://localhost:8787 bun run build)

# 3. the host's backend — given the embed key, and not the signing key
CRONOS_URL=http://localhost:8787 CRONOS_EMBED_KEY="$CRONOS_EMBED_KEY" bun integrations/server.ts
```

Open http://localhost:5301/plain/ or http://localhost:5301/react/. Sign in as
Alice, then as Bob: the same report, each with their own customer's invoices.

## How it fits together

```
  browser                     your backend (server.ts)              cronos
     │  POST /api/report-token      │                                  │
     │ ───────────────────────────> │ session → user → customer_id     │
     │        { token, expiresAt }  │ sign with CRONOS_EMBED_KEY       │
     │ <─────────────────────────── │                                  │
     │                                                                 │
     │  GET  /v1/embed/cronos-embed.js   (the element, once)           │
     │ ──────────────────────────────────────────────────────────────> │
     │  POST /v1/embed/reports/billing-summary   Authorization: Bearer │
     │ ──────────────────────────────────────────────────────────────> │
     │                       rows for that customer_id, and no others  │
     │ <────────────────────────────────────────────────────────────── │
```

Your backend never calls cronos, and cronos never sees your user table. The
token is the whole contract between them.

## The plain site

[`plain/index.html`](plain/index.html) loads the element with one tag, from
the cronos server:

```html
<script type="module" src="https://reports.acme.com/v1/embed/cronos-embed.js"></script>
```

and [`plain/app.js`](plain/app.js) creates it once it has a token:

```js
const { token, expiresAt } = await fetch('/api/report-token', { method: 'POST' }).then((r) => r.json())

const report = document.createElement('cronos-report')
report.setAttribute('endpoint', 'https://reports.acme.com')
report.setAttribute('report', 'billing-summary')
report.setAttribute('token', token)
document.querySelector('main').append(report)

// Filters are a property, because they are structured:
report.filters = { period: { op: 'between', values: ['2026-01-01', '2026-12-31'] } }
report.addEventListener('cronos:error', (e) => console.warn(e.detail.message))
```

The same three lines work in Vue, Svelte and Angular — it is a standard custom
element.

## The React site

[`react/src/App.tsx`](react/src/App.tsx):

```tsx
import { CronosReport } from '@cronos/react'

const token = useReportToken(session.user)   // asks the backend, renews before expiry

{token && (
  <CronosReport
    endpoint={CRONOS_URL}
    token={token}
    report="billing-summary"
    onError={(e) => setProblem(e.message)}
  />
)}
```

`@cronos/react` carries no copy of the element. It imports it from `endpoint`
the first time a report mounts, so it is always the build that matches the
server.

## What makes it safe

Each of these is in the code with a comment where it happens, and
`live-integrations.sh` asserts every one against a running server.

- **The scope is decided on the server.** `/api/report-token` takes nothing
  from the request but the session. A visitor who edits the page, the request
  or the token gets their own rows or a refusal.
- **The host holds the embed key only.** `CRONOS_EMBED_KEY` signs embed tokens
  and nothing else; the check signs a portal session with it and cronos
  refuses it. The signing key is not in this process's environment at all.
- **Tokens are short.** Fifteen minutes, renewed at four fifths. cronos refuses
  one that lives longer than twenty-four hours, whoever signed it.
- **A strict Content-Security-Policy.** `script-src 'self' <cronos>`,
  `connect-src 'self' <cronos>`, nothing inline. Both pages run under it with
  nothing blocked.
- **Where cronos is, is a constant.** `CRONOS_URL` on the server and
  `VITE_CRONOS_URL` at build time — never a query parameter. It decides what
  code the page runs.
- **The token response is `no-store`**, the session cookie is `HttpOnly`, and
  the token never appears in a URL.

## Making it yours

1. Port `token.ts` to your backend's language. It is a JSON payload, base64url,
   and one HMAC-SHA256.
2. Replace the demo session and `CUSTOMER_OF` with your own: who is signed in,
   and which rows that is.
3. Put your application's origin in `CRONOS_ORIGINS`, and the cronos origin in
   your CSP.
4. Keep `CRONOS_EMBED_KEY` where your other server secrets are.
