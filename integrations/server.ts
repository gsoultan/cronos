/*
 * A host application's backend, as small as it can be and still be right.
 *
 * It does the three things cronos cannot do for you:
 *
 *   1. knows who is signed in            — the session below
 *   2. decides which rows they may read  — CUSTOMER_OF
 *   3. signs a token that says so        — POST /api/report-token
 *
 * and serves two front ends that are otherwise identical: /plain/ is HTML and
 * a script, /react/ is a Vite + React application. Both load the report
 * element from the cronos server itself, so neither installs it.
 *
 *   CRONOS_URL=http://localhost:8787 CRONOS_EMBED_KEY=… bun integrations/server.ts
 */
import { randomUUID } from 'node:crypto'
import { join, normalize } from 'node:path'
import { mintEmbedToken } from './token'

/* A constant of the deployment, read once on the server. It decides which
   code the page runs, so it is never taken from a request. */
const CRONOS_URL = (process.env.CRONOS_URL ?? 'http://localhost:8787').replace(/\/$/, '')
const EMBED_KEY = process.env.CRONOS_EMBED_KEY ?? ''
const ORG = process.env.CRONOS_ORG ?? 'acme'
const PROJECT = process.env.CRONOS_PROJECT ?? 'finance'
const REPORT = 'billing-summary'
const PORT = Number(process.env.PORT ?? 5301)
const ROOT = import.meta.dir

if (!EMBED_KEY) {
  console.error('CRONOS_EMBED_KEY is required: the key your cronos server verifies embed tokens with')
  process.exit(1)
}

/* Your user table. The mapping from a person to the rows they may read is
   yours, and it is the only place it exists: cronos never sees it, and the
   browser is never asked. */
const CUSTOMER_OF: Record<string, string> = { alice: 'c-1', bob: 'c-2' }

/* Sessions. A real application already has these — this is here so the
   example has something honest to read the user from. Bounded, because
   anybody can press Sign in. */
const sessions = new Map<string, string>()
const MAX_SESSIONS = 1000

function signIn(user: string): string {
  if (sessions.size >= MAX_SESSIONS) sessions.delete(sessions.keys().next().value!)
  const id = randomUUID()
  sessions.set(id, user)
  return id
}

function sessionOf(req: Request): { id: string; user: string } | null {
  const id = /(?:^|;\s*)sid=([^;]+)/.exec(req.headers.get('cookie') ?? '')?.[1]
  const user = id ? sessions.get(id) : undefined
  return id && user ? { id, user } : null
}

/*
 * The policy a page that embeds cronos needs, and nothing looser.
 *
 * script-src and connect-src name the cronos server: the first lets the page
 * load the element from it, the second lets the element call it. Nothing is
 * inline. This is the control that still holds if something else goes wrong —
 * a script injected into the page cannot load code from, or send a token to,
 * anywhere that is not listed here.
 */
const CSP = [
  `default-src 'self'`,
  `script-src 'self' ${CRONOS_URL}`,
  `connect-src 'self' ${CRONOS_URL}`,
  `style-src 'self'`,
  `img-src 'self' data:`,
  `object-src 'none'`,
  `base-uri 'none'`,
  `frame-ancestors 'none'`,
].join('; ')

const TYPES: Record<string, string> = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
}

async function page(dir: string, path: string): Promise<Response> {
  const file = normalize(path === '' || path.endsWith('/') ? path + 'index.html' : path)
  if (file.startsWith('..')) return new Response('Not found', { status: 404 })
  const found = Bun.file(join(ROOT, dir, file))
  if (!(await found.exists())) return new Response('Not found', { status: 404 })

  const type = TYPES[file.slice(file.lastIndexOf('.'))] ?? 'application/octet-stream'
  const headers = { 'content-type': type, 'content-security-policy': CSP, 'x-content-type-options': 'nosniff' }
  if (!type.startsWith('text/html')) return new Response(found, { headers })
  /* The one thing the page is told: where cronos is. Written into the HTML by
     the server so the script tag can name it, rather than assembled in the
     browser from something a visitor could change. */
  return new Response((await found.text()).replaceAll('__CRONOS_URL__', CRONOS_URL), { headers })
}

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    // A token is a credential. Nothing between here and the browser keeps it.
    headers: { 'content-type': 'application/json', 'cache-control': 'no-store' },
  })

Bun.serve({
  port: PORT,
  async fetch(req) {
    const url = new URL(req.url)

    if (url.pathname === '/api/session' && req.method === 'GET') {
      return json({ user: sessionOf(req)?.user ?? null, users: Object.keys(CUSTOMER_OF) })
    }

    if (url.pathname === '/api/sign-in' && req.method === 'POST') {
      const { user } = (await req.json().catch(() => ({}))) as { user?: string }
      if (!user || !(user in CUSTOMER_OF)) return json({ error: 'Unknown user.' }, 400)
      const res = json({ user })
      res.headers.set('set-cookie', `sid=${signIn(user)}; HttpOnly; SameSite=Lax; Path=/`)
      return res
    }

    if (url.pathname === '/api/sign-out' && req.method === 'POST') {
      const session = sessionOf(req)
      if (session) sessions.delete(session.id)
      const res = json({ user: null })
      res.headers.set('set-cookie', 'sid=; HttpOnly; SameSite=Lax; Path=/; Max-Age=0')
      return res
    }

    /*
     * The route this example exists for.
     *
     * Nothing in the request chooses the scope. The session says who is
     * asking, the table above says which customer that is, and the token
     * carries it signed — so a visitor who edits the page, the request or the
     * token gets their own rows or a refusal, never somebody else's.
     */
    if (url.pathname === '/api/report-token' && req.method === 'POST') {
      const session = sessionOf(req)
      if (!session) return json({ error: 'Sign in first.' }, 401)
      return json(mintEmbedToken(EMBED_KEY, {
        org: ORG,
        project: PROJECT,
        subject: session.user,
        report: REPORT,
        scope: { customer_id: CUSTOMER_OF[session.user]! },
      }))
    }

    if (url.pathname === '/') return Response.redirect('/plain/', 302)
    if (url.pathname.startsWith('/plain/')) return page('plain', url.pathname.slice('/plain/'.length))
    if (url.pathname.startsWith('/react/')) return page('react/dist', url.pathname.slice('/react/'.length))
    return new Response('Not found', { status: 404 })
  },
})

console.log(`host application on http://localhost:${PORT}  (/plain/ and /react/), cronos at ${CRONOS_URL}`)
