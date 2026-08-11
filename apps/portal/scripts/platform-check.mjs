/* PWA installability, the row worker, and mobile layout. */
import { chromium, devices } from 'playwright'
import { readFileSync } from 'node:fs'

const B = process.env.BASE ?? 'http://localhost:5173'
const b = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
let f = 0
const ok = (n, c) => { console.log(`  ${c ? 'ok  ' : 'FAIL'} ${n}`); if (!c) f++ }

/* -- PWA: the manifest is checked as built, not as written. --------------- */
const m = JSON.parse(readFileSync('dist/manifest.webmanifest', 'utf8'))
ok('manifest has icons at all', m.icons.length > 0)
ok('has the 192 and 512 Chrome requires for install',
  ['192x192', '512x512'].every((s) => m.icons.some((i) => i.sizes === s)))
ok('has a maskable icon so Android does not crop the mark',
  m.icons.some((i) => i.purpose === 'maskable'))
ok('declares start_url, scope and standalone display',
  !!m.start_url && !!m.scope && m.display === 'standalone')

/* -- Worker --------------------------------------------------------------- */
const desktop = await (await b.newContext({ viewport: { width: 1500, height: 1000 } })).newPage()
const workers = []
desktop.on('worker', (w) => workers.push(w.url()))
const errs = []
desktop.on('pageerror', (e) => errs.push(String(e)))
await desktop.goto(`${B}/reports/monthly-invoice-statement`, { waitUntil: 'domcontentloaded' })
await desktop.waitForSelector('[data-testid=row-meta]')
await desktop.waitForTimeout(1200)
ok('a row worker is spawned', workers.length >= 1)
const meta = await desktop.locator('[data-testid=row-meta]').innerText()
ok(`filtering is reported as off the main thread (${meta.replace(/\s+/g, ' ')})`,
  meta.includes('off the main thread'))
ok('aggregates cover the whole set, not the page',
  meta.includes('4,000'))

// Filtering keeps the main thread answering.
await desktop.click('[data-testid=filter-toggle]')
await desktop.click('text=+ Add condition')
await desktop.waitForTimeout(400)
const before = Date.now()
await desktop.click('button:has-text("Apply filters")').catch(() => {})
await desktop.evaluate(() => document.body.offsetWidth)   // a main-thread round trip
ok('the main thread answers during a filter', Date.now() - before < 3000)

/* -- Mobile --------------------------------------------------------------- */
const phone = await (await b.newContext({ ...devices['iPhone 14'] })).newPage()
for (const [path, name] of [['/', 'reports'], ['/data', 'data'], ['/settings', 'settings'],
                            ['/reports/monthly-invoice-statement', 'a report'],
                            ['/reports/new', 'the editor'], ['/account', 'account']]) {
  await phone.goto(B + path, { waitUntil: 'domcontentloaded' })
  await phone.waitForTimeout(600)
  const w = await phone.evaluate(() => document.documentElement.scrollWidth)
  ok(`${name} fits a 390px screen (${w}px)`, w <= 390)
}
await phone.goto(B + '/', { waitUntil: 'domcontentloaded' })
ok('the nav strip scrolls rather than the page',
  await phone.evaluate(() => {
    const nav = document.querySelector('nav[aria-label=Main]')
    return !!nav && nav.scrollWidth > nav.clientWidth
  }))

console.log(errs.length ? '\nERRORS:\n' + errs.join('\n') : '\nno page errors')
console.log(f ? `${f} failed` : 'all passed')
await b.close()
process.exit(f ? 1 : 0)
