/* Search and paging on the Data page. */
import { chromium } from 'playwright'
const B = process.env.BASE ?? 'http://localhost:5173'
const b = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const p = await (await b.newContext({ viewport: { width: 1600, height: 1100 } })).newPage()
const errs = []
p.on('pageerror', e => errs.push(String(e)))
p.on('console', m => m.type() === 'error' && errs.push(m.text()))
let f = 0
const ok = (n, c) => { console.log(`  ${c ? 'ok  ' : 'FAIL'} ${n}`); if (!c) f++ }

const rows = (card) => p.locator(`[data-testid=${card}-card] li`)
const search = p.locator('input[aria-label="Search sources and datasets"]')

await p.goto(B + '/data', { waitUntil: 'domcontentloaded' })
await p.waitForSelector('[data-testid=sources-card]')

ok('a page holds six sources', await rows('sources').count() === 6)
ok('and six datasets', await rows('datasets').count() === 6)
ok('the range is stated', await p.locator('text=1–6 of 14 sources').isVisible())

await p.click('[data-testid=sources-card] button:has-text("Next")')
await p.waitForTimeout(250)
ok('next advances the sources list only',
  await p.locator('text=7–12 of 14 sources').isVisible() &&
  await p.locator('text=1–6 of 12 datasets').isVisible())
await p.click('[data-testid=sources-card] button:has-text("Next")')
await p.waitForTimeout(250)
ok('the last page is short and Next stops',
  await rows('sources').count() === 2 &&
  await p.locator('[data-testid=sources-card] button:has-text("Next")').isDisabled())

// Searching from a deep page must not strand you on an empty one.
await search.fill('lake')
await p.waitForTimeout(300)
ok('search resets to the first page',
  await p.locator('text=/1–\\d+ of/').first().isVisible() || await rows('sources').count() > 0)
ok('search filters sources', await rows('sources').count() === 3)
ok('one box searches both lists', await rows('datasets').count() === 0)
ok('the counts are shown while searching',
  await p.locator('text=3 sources · 0 datasets').isVisible())
ok('the empty list says why rather than vanishing',
  await p.locator('text=No datasets match').isVisible())

await search.fill('carrier')
await p.waitForTimeout(300)
ok('search reaches descriptions and source names', await rows('datasets').count() >= 1)

await search.fill('zzzz')
await p.waitForTimeout(300)
ok('no matches at all gets one empty state, not two',
  await p.locator('text=Nothing matches').isVisible() &&
  await p.locator('[data-testid=sources-card]').count() === 0)

await p.click('button:has-text("Clear search")')
await p.waitForTimeout(300)
ok('clearing restores everything', await rows('sources').count() === 6)

// Chrome only when it earns its place.
await search.fill('warehouse')
await p.waitForTimeout(300)
ok('no pagination controls under a short list',
  await p.locator('[data-testid=sources-card] button:has-text("Next")').count() === 0)

console.log(errs.length ? '\nERRORS:\n' + errs.join('\n') : '\nno console errors')
console.log(f ? `${f} failed` : 'all passed')
await b.close()
process.exit(f ? 1 : 0)
