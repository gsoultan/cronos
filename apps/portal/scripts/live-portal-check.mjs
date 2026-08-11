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

/* -- The edit path -------------------------------------------------------
   The round trip nothing else can prove: the server's stored bytes, read into
   a form, written back, and stored again. A serialiser tested against its own
   parser proves only that the two agree. */

await page.goto(`${B}/data/datasets/active-customers/edit`, { waitUntil: 'domcontentloaded' })
await page.locator('text=API name').waitFor({ timeout: 15000 })
const edit = await page.locator('body').innerText()
ok('the form opens on the stored definition', edit.includes('active-customers'))

/* Everything a populated form holds is a control's value, which innerText does
   not reach — an empty form and a filled one read identically from the outside.
   So these ask the controls what they hold. */
const written = await page.locator('textarea').evaluateAll((n) => n.map((e) => e.value))
ok('and shows the stored query, not an empty canvas',
  written.some((v) => v.includes('SELECT id, name, city FROM customers')))
ok('and the description the file carries',
  written.some((v) => v.includes('Who receives a statement')))

/* Read out of the file: customers.yaml declares its fields as flow mappings,
   which is the syntax every hand-written dataset here uses. Behind the Fields
   tab, because the form opens on the query. */
await page.locator('button:has-text("Fields")').click()
const values = await page.locator('input').evaluateAll((n) => n.map((e) => e.value))
ok('and the fields the file declares, labels included',
  ['Customer', 'Name', 'City'].every((label) => values.includes(label)))
/* Nothing in this file is outside what the form models, so nothing is
   threatened — the warning must stay quiet or it means nothing when it fires. */
ok('and warns about nothing, because nothing would be lost',
  await page.locator('[data-testid=unmodelled-warning]').count() === 0)

await page.locator('button:has-text("Save dataset")').click()
await page.waitForURL(/\/data$/, { timeout: 15000 })
ok('saving returns to the catalogue', page.url().endsWith('/data'))

/* The definition the server now holds. Byte-for-byte equality is not the
   claim — the form rewrites flow mappings as blocks — but every value the file
   carried must still be there, and the report that reads it must still run. */
const stored = await fetch(`${process.env.API}/v1/definitions/Dataset/active-customers`, {
  headers: { authorization: `Bearer ${process.env.TOKEN}` },
}).then((r) => r.text())

ok('the saved definition kept its query', stored.includes('SELECT id, name, city FROM customers'))
ok('and its field labels', stored.includes('Customer') && stored.includes('City'))
ok('and its source', stored.includes('warehouse'))

/* The proof that the rewrite is valid and not merely well-formed: the schedule
   bursts over this dataset, and the report still renders. */
await page.goto(`${B}/reports/billing-summary`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=live-report]').waitFor({ timeout: 20000 })
ok('and the report that reads it still runs',
  (await page.locator('[data-testid=live-report]').innerText()).includes('154,651.50'))

/* -- What the form cannot show, it says so about --------------------------
   billing-summary has a block filter and a block sort, neither of which the
   builder draws. Saving would drop them, and the author is told which. */
await page.goto(`${B}/reports/billing-summary/edit`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=unmodelled-warning]').waitFor({ timeout: 15000 })
const warned = await page.locator('[data-testid=unmodelled-warning]').innerText()
ok('an unmodelled block filter is named rather than dropped in silence',
  warned.includes('layout[1].filter'))
ok('and so is a block sort', warned.includes('layout[3].sort'))
/* The shared filter is not in that list: it is a spec key the form never
   writes, so a save folds it back untouched. */
ok('a spec the form never writes is carried, not warned about',
  !warned.includes('spec.filters'))

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
process.exit(fails ? 1 : 0)
