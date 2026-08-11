/*
 * The portal against a real cronosd.
 *
 * Every other suite runs on sample data. That is deliberate — it is what makes
 * the interface workable before a server exists — but it also means none of
 * them would notice if the API contract moved underneath. This one would.
 */
import { chromium } from 'playwright'

const B = process.env.BASE ?? 'http://localhost:5174'
const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const page = await browser.newPage()
let fails = 0
const ok = (name, cond) => { console.log(`  ${cond ? 'ok  ' : 'FAIL'} ${name}`); if (!cond) fails++ }

const errors = []
page.on('pageerror', (e) => errors.push(String(e)))

await page.goto(`${B}/reports/billing-summary`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=live-report]').waitFor({ timeout: 20000 })
const body = await page.locator('[data-testid=live-report]').innerText()

/* -- It is the server's numbers, not the fixture's ------------------------ */

/* Every customer's invoices, not one's. An author is a project member, so row
   scope does not apply to them — it isolates an ISV's end customers from each
   other, and an author who could not preview their own report would be looking
   at a page of em dashes. The embed check asserts the other half: the same
   report through an embed token returns 60,171.25 for c-1 alone. */
ok('an author sees the whole project (154,651.50)', body.includes('154,651.50'))
ok('a block filter narrows one tile only (24,720.75 outstanding)',
  body.includes('24,720.75'))
/* The chart shortens a month for an axis, and the table uppercases a heading
   in CSS — innerText returns what is rendered, so both assertions read the
   text a person sees rather than the string the server sent. */
ok('the chart labels months rather than truncated dates',
  body.includes("Jul \u201926") && !body.includes('2026-07-01'))
ok('table headings come from the dataset labels',
  body.includes('ISSUED') && body.includes('AMOUNT'))

/* The engine formatted these. A table that formats them again meets
   Number("19,800") and prints NaN down the whole column. */
ok('amounts are not reformatted into NaN', !body.includes('NaN'))

/* The fixture's figures must be nowhere near it. */
ok('the sample figures are gone', !body.includes('49.9M'))

/* -- The report format's promise, computed by the server ------------------ */
ok('a block says the filter that misses it does not apply',
  (await page.locator('[data-testid=unaffected]').first().innerText()).includes('Region'))

/* -- Connected mode is announced as connected ----------------------------- */
ok('no sample-data banner when connected',
  await page.locator('[data-testid=sample-banner]').count() === 0)

/* -- The catalogue: what the project contains ----------------------------- */
await page.goto(`${B}/data`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=datasets-card]').waitFor({ timeout: 15000 })
const data = await page.locator('body').innerText()

ok('the data page lists the datasets the server has',
  data.includes('invoices') && data.includes('active-customers') && data.includes('statement-lines'))
ok('and the source they read', data.includes('warehouse'))
/* The limits are what nobody opens the file for: what one query may spend, and
   how much it may hand back. */
ok('a source shows its limits', data.includes('30s') && data.includes('100,000'))
/* Whether an embedded end customer sees only their own rows. */
ok('a row-scoped dataset says so', data.includes('row scoped'))
/* And the sample fixture is nowhere near it. */
ok('the sample sources are gone', !data.includes('Northwind'))

await page.goto(`${B}/schedules`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=schedules-card]').waitFor({ timeout: 15000 })
const schedules = await page.locator('body').innerText()
ok('the schedules page lists the real schedule', schedules.includes('monthly-statements'))
/* Computed from the cron expression in its own timezone, by the loop that will
   honour it — which is the one column a fixture cannot have. */
ok('and when it next fires',
  (await page.locator('[data-testid=next-run]').first().innerText()).startsWith('next '))

/* -- A report nobody has says so in the server's words -------------------- */
await page.goto(`${B}/reports/not-a-report`, { waitUntil: 'domcontentloaded' })
await page.locator('text=could not be run').waitFor({ timeout: 15000 })
ok('an unknown report reports the server\'s message',
  (await page.locator('body').innerText()).includes('No such report'))

ok(`nothing was thrown (${errors.length})`, errors.length === 0)
if (errors.length) console.log(errors.slice(0, 3).map((e) => `       ${e}`).join('\n'))

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
process.exit(fails ? 1 : 0)
