/*
 * Both example sites, in a real browser, against a real cronosd.
 *
 * The examples are documentation, and documentation that is not run is wrong
 * by the second release. Started by scripts/live-integrations.sh, which builds
 * and starts cronosd and the host backend.
 *
 * It is also where the embed key is proved end to end: the host holds
 * CRONOS_EMBED_KEY and nothing else, and the last section signs with it what
 * a compromised host would.
 */
import { createHmac } from 'node:crypto'
import { chromium } from 'playwright'

const HOST = process.env.HOST_URL
const API = process.env.CRONOS_URL
const EMBED_KEY = process.env.CRONOS_EMBED_KEY
const SIGNING_KEY = process.env.CRONOS_SIGNING_KEY

if (!HOST || !API || !EMBED_KEY || !SIGNING_KEY) {
  console.error('run this through scripts/live-integrations.sh')
  process.exit(2)
}

let fails = 0
const ok = (name, cond) => { console.log(`  ${cond ? 'ok  ' : 'FAIL'} ${name}`); if (!cond) fails++ }

const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
process.on('exit', () => { browser.close() })

/* Alice is customer c-1 and Bob is c-2 in demo/seed.sql; the totals are what
   the engine computes for each. */
const ALICE = '60,171.25'
const BOB = '78,950.25'

for (const site of ['plain', 'react']) {
  console.log(`\n/${site}/`)
  const context = await browser.newContext()
  const page = await context.newPage()

  /* A policy violation is reported to the console and otherwise silent: the
     page simply lacks whatever was blocked. So every one is collected. */
  const blocked = []
  page.on('console', (m) => { if (/Content Security Policy|Refused to/i.test(m.text())) blocked.push(m.text()) })
  const requested = []
  page.on('request', (r) => requested.push(r.url()))

  const report = page.locator('cronos-report')
  const text = () => page.evaluate(() => document.querySelector('cronos-report')?.shadowRoot?.textContent ?? '')
  const shows = (total) => page.waitForFunction(
    (t) => document.querySelector('cronos-report')?.shadowRoot?.textContent?.includes(t), total, { timeout: 15000 })

  const page1 = await page.goto(`${HOST}/${site}/`, { waitUntil: 'domcontentloaded' })
  await page.locator('[data-testid=sign-in-alice]').waitFor()
  ok('signed out, there is no report and no token to ask for',
    (await report.count()) === 0 &&
    (await context.request.post(`${HOST}/api/report-token`, { data: {} })).status() === 401)

  await page.click('[data-testid=sign-in-alice]')
  await shows(ALICE)
  ok(`Alice sees her own total (${ALICE})`, (await text()).includes(ALICE))
  ok('the element came from the cronos server, not from the site',
    requested.includes(`${API}/v1/embed/cronos-embed.js`))

  await page.click('[data-testid=sign-out]')
  await page.locator('[data-testid=sign-in-bob]').waitFor()
  await page.click('[data-testid=sign-in-bob]')
  await shows(BOB)
  const bob = await text()
  ok(`Bob sees his (${BOB})`, bob.includes(BOB))
  ok("and never Alice's", !bob.includes(ALICE))

  const csp = page1.headers()['content-security-policy'] ?? ''
  ok('under a policy that names cronos and allows nothing inline',
    csp.includes(`script-src 'self' ${API}`) && !csp.includes('unsafe-inline'))
  ok(`and nothing was blocked by it (${blocked.length})`, blocked.length === 0)
  if (blocked.length) console.log(blocked.slice(0, 3).map((b) => `       ${b}`).join('\n'))

  await context.close()
}

/* -- What the host's key is worth ------------------------------------------ */
console.log('\nthe embed key')

const sign = (key, claims) => {
  const now = Math.floor(Date.now() / 1000)
  const body = 'v1.' + Buffer.from(JSON.stringify({ iat: now, exp: now + 600, ...claims })).toString('base64url')
  return `${body}.${createHmac('sha256', key).update(body).digest('base64url')}`
}
const tenancy = { org: 'acme', prj: 'finance', sub: 'host-backend' }
const get = (path, token) => fetch(`${API}${path}`, { headers: { authorization: `Bearer ${token}` } })
const open = (token) => fetch(`${API}/v1/embed/reports/billing-summary`, {
  method: 'POST',
  headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json' },
  body: '{"filters":{}}',
})

/* The session a host that held the old, single key could sign for itself. */
const admin = { ...tenancy, aud: 'portal', rol: 'admin', org_rol: 'owner', plt: true }
ok('a portal session signed with the signing key opens the catalogue (the control)',
  (await get('/v1/catalog', sign(SIGNING_KEY, admin))).status === 200)
ok('the same session signed with the embed key is refused',
  (await get('/v1/catalog', sign(EMBED_KEY, admin))).status === 401)

const reader = { ...tenancy, aud: 'embed', rpt: 'billing-summary', scp: { customer_id: 'c-1' } }
ok('the embed key opens the report it is for',
  (await open(sign(EMBED_KEY, reader))).status === 200)
ok('a token signed for ten years is refused',
  (await open(sign(EMBED_KEY, { ...reader, exp: Math.floor(Date.now() / 1000) + 10 * 365 * 86400 }))).status === 401)
ok('and one that does not say when it was issued',
  (await open(sign(EMBED_KEY, { ...reader, iat: 0 }))).status === 401)

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
process.exit(fails ? 1 : 0)
