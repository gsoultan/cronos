/*
 * Two people, one browser.
 *
 * A shared machine, or a support engineer with two accounts, or an ISV's own
 * staff moving between the customers they host: sign out, sign in as somebody
 * in another organisation, without reloading the page. That is one page load
 * with two sessions in it, and the portal's cache is per page load.
 *
 * It found what it was written to look for. Every query key names what was
 * asked for and none of them name who asked — `['catalog']`, `['people']`,
 * `['runs', 50]` — so the second person's Reports page listed the first
 * person's reports, under the second person's organisation in the header. With
 * `staleTime` on the query no request was even sent; the cache answered. There
 * is no way to see that from the server's access log, and nothing in the
 * interface says anything is wrong.
 *
 *   PORTAL=… API=… node e2e/handover.mjs
 *
 * Driven by scripts/live-handover.sh, which stands up the two projects.
 */
import { chromium } from 'playwright'

const PORTAL = process.env.PORTAL ?? 'http://localhost:5273'
const ok = (m) => console.log(`  \x1b[32mok\x1b[0m ${m}`)
const die = (m) => { console.error(`  \x1b[31mFAILED\x1b[0m ${m}`); process.exit(1) }

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1400, height: 1000 } })

/* Every request that actually left the browser, so "it refetched" can be told
   apart from "the cache answered" — which is the whole distinction here. */
const asked = []
page.on('response', (r) => {
  const u = new URL(r.url())
  if (u.pathname.startsWith('/v1/')) asked.push(u.pathname)
})

async function signIn(email, password) {
  await page.getByTestId('email').fill(email)
  await page.getByTestId('password').fill(password)
  await Promise.all([
    page.waitForResponse((r) => r.url().includes('/v1/auth/login')),
    page.getByTestId('submit').click(),
  ])
  await page.waitForTimeout(2000)
}

const text = () => page.locator('body').innerText()

await page.goto(PORTAL + '/', { waitUntil: 'networkidle' })

// -- Ada, in acme/finance ----------------------------------------------------

await signIn('ada@acme.example', 'a-password-for-ada')
const ada = await text()
if (!/Billing summary/.test(ada)) die(`ada cannot see her own reports: ${ada.slice(0, 200)}`)
ok('ada sees acme/finance — Billing summary')

// Load the pages that cache the most: the roster and the run history.
for (const [path, want] of [['/data', /Datasets|Sources/], ['/activity', /Activity/]]) {
  await page.goto(PORTAL + path, { waitUntil: 'networkidle' })
  await page.waitForTimeout(1200)
  if (!want.test(await text())) die(`ada could not load ${path}`)
}
await page.goto(PORTAL + '/', { waitUntil: 'networkidle' })
await page.waitForTimeout(800)
ok('and has loaded the pages that cache — data, activity')

// -- Handed over -------------------------------------------------------------

await page.getByTestId('sign-out').click()
await page.waitForTimeout(2000)
if (await page.getByTestId('email').count() !== 1) die('signing out did not reach the sign-in page')
ok('signs out, and the sign-in page comes back')

// No reload. A reload would empty the cache by emptying the tab, which is
// exactly the thing this is here to not rely on.
if (page.url().replace(PORTAL, '').split('?')[0] !== '/') die(`sign-out navigated to ${page.url()}`)

asked.length = 0
await signIn('rin@globex.example', 'a-password-for-rin')
const rin = await text()

// -- What Rin is shown -------------------------------------------------------

for (const acmes of ['Billing summary', 'Customer statement', 'Customers']) {
  if (rin.includes(acmes)) {
    await page.screenshot({ path: '/tmp/cronos-handover-leak.png', fullPage: true })
    die(`globex/ops is shown acme/finance's "${acmes}" — see /tmp/cronos-handover-leak.png`)
  }
}
ok('globex/ops is shown none of acme/finance’s reports')

if (!/Globex only/.test(rin)) die(`globex/ops cannot see its own report: ${rin.slice(0, 200)}`)
ok('and does see its own')

// The cache being empty is only half of it: the point is that the question is
// asked again rather than answered from what the last session left behind.
if (!asked.some((p) => p === '/v1/catalog')) {
  die(`the catalogue was not re-fetched for the second session — asked: ${asked.join(' ')}`)
}
ok('the catalogue was fetched again rather than remembered')

console.log('\n  \x1b[32mthe cache belongs to the session\x1b[0m\n')
await browser.close()
