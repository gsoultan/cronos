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

/* -- The builder shows the whole report ----------------------------------
   billing-summary has a block filter and a block sort, which the builder used
   to be unable to draw — so opening it warned that saving would drop them.
   Both are editable now, and the warning has nothing left to say. */
await page.goto(`${B}/reports/billing-summary/edit`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=canvas-block]').first().waitFor({ timeout: 20000 })
ok('nothing in this report is beyond the editor',
  await page.locator('[data-testid=unmodelled-warning]').count() === 0)

/* The inspector is the block when a block is selected, so the predicate is
   read from the block that carries it. billing-summary's second stat is the
   one filtered to overdue. */
const blocks = page.locator('[data-testid=canvas-block]')
let found = ''
for (let i = 0; i < await blocks.count(); i++) {
  await blocks.nth(i).click()
  await page.locator('[data-testid=block-filter]').waitFor({ timeout: 10000 })
  const value = await page.locator('[data-testid=block-filter]').inputValue()
  if (value) found = value
}
ok('and the block filter is there to read and change', found.includes("status = 'overdue'"))

/* Saving it back keeps both. The warning being silent is only worth anything
   if it is telling the truth. */
await page.locator('button:has-text("Save report")').click()
await page.waitForURL(/\/$/, { timeout: 15000 })
const saved = await fetch(`${process.env.API}/v1/definitions/Report/billing-summary`, {
  headers: { authorization: `Bearer ${process.env.TOKEN}` },
}).then((r) => r.text())
ok('and a save keeps the filter', saved.includes("status = 'overdue'"))
ok('and the sort', saved.includes('issued_at'))
ok('and the shared filter the form never writes', saved.includes('name: region'))

/* -- Activity: what ran, and who got it ----------------------------------
   Nothing has run yet, and the page says so rather than showing an empty
   table that could equally mean a broken query. */

await page.goto(`${B}/activity`, { waitUntil: 'domcontentloaded' })
await page.locator('text=Activity').first().waitFor({ timeout: 15000 })
ok('an empty history says nothing has run',
  (await page.locator('body').innerText()).includes('Nothing has run yet'))

/* Then run one. A monthly schedule is otherwise untestable until the first of
   the month, and this is the assertion that proves the whole path: the
   scheduler renders the report, bursts it per customer, delivers each one and
   records what happened — none of which any unit test covers together. */
await page.goto(`${B}/schedules`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=run-now]').first().waitFor({ timeout: 15000 })
await page.locator('[data-testid=run-now]').first().click()
await page.locator('[data-testid=run-confirm]').first().click()

await page.locator('[data-testid=runs-card]').waitFor({ timeout: 30000 })
ok('running a schedule lands on its record', page.url().endsWith('/activity'))

const activity = await page.locator('body').innerText()
ok('the record names the schedule that ran', activity.includes('monthly-statements'))
/* The demo bursts one statement per customer, and seed.sql has three. A run
   that says 1 of 1 would mean the burst did not burst. */
ok('and how many of how many it delivered',
  (await page.locator('[data-testid=run-count]').first().innerText()).includes(' of 3'))
ok('and whether they arrived',
  ['Delivered', 'failed'].some((s) => activity.includes(s)))
/* Reproducibility: a run record names the exact definition it ran, because
   "what did the customer actually receive" is answerable only against those
   bytes. */
ok('and the version of the report it ran', /[0-9a-f]{12}/.test(activity))

/* Not while it is still sending: a run in flight has however many deliveries
   have been written so far, and asserting on that count is asserting on how
   fast the machine is. */
await page.locator('[data-testid=run-status]').first()
  .filter({ hasNotText: 'Sending' }).waitFor({ timeout: 30000 })

await page.locator('[data-testid=run-toggle]').first().click()
/* Waiting for the last row rather than for the element, because the list
   renders as soon as the query resolves and reading it then reads whatever
   React had committed by that instant. */
await page.locator('[data-testid=run-deliveries] li').nth(2).waitFor({ timeout: 15000 })
const recipients = await page.locator('[data-testid=run-deliveries]').innerText()
/* All three, not just the first. A burst that delivered one document is a
   burst that did not burst. */
ok('and every recipient it attempted',
  ['c-1', 'c-2', 'c-3'].every((who) => recipients.includes(who)))
/* The demo delivers to files, so the destination is a path — whichever channel
   it is, a delivery record that cannot say where it went is not one. */
ok('with where each one went', recipients.includes('statement'))

/* -- Sharing -------------------------------------------------------------
   A report handed to somebody with no account here. The whole token design
   exists for this, and until now nothing minted one. */

/* billing-summary reads a row-scoped dataset, and the recipient of a share
   holds an embed token — which docs/tenancy.md says row scope applies to. So
   the link would either show nothing or, if the token quietly claimed project
   membership, show every customer's rows to whoever it was forwarded to.
   Refused, and the message says which dataset and why. */
await page.goto(`${B}/reports/billing-summary`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=share-button]').waitFor({ timeout: 20000 })
await page.locator('[data-testid=share-button]').click()
await page.locator('[role=tab]:has-text("Get a link")').click()
await page.locator('[data-testid=audience-anyone]').click()
await page.locator('[data-testid=create-link]').click()
await page.locator('[data-testid=share-error]').waitFor({ timeout: 15000 })
const refused = await page.locator('[data-testid=share-error]').innerText()
ok('a report whose rows belong to one customer cannot be shared to anyone',
  refused.includes('scoped per customer') || refused.includes('one customer at a time'))
ok('and the refusal names the dataset', refused.includes('invoices'))

/* customer-overview reads a dataset that is not row-scoped, so a link to it
   shows the same thing to everybody — which is what a link to it means. */
await page.goto(`${B}/reports/customer-overview`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=share-button]').waitFor({ timeout: 20000 })
await page.locator('[data-testid=share-button]').click()
await page.locator('[role=tab]:has-text("Get a link")').click()
await page.locator('[data-testid=audience-anyone]').click()

ok('no link exists until somebody asks for one',
  await page.locator('[data-testid=share-link]').count() === 0)
await page.locator('[data-testid=create-link]').click()
await page.locator('[data-testid=share-link]').waitFor({ timeout: 15000 })
const shareUrl = await page.locator('[data-testid=share-link]').inputValue()
ok('and asking records one', /\/s\/shr_[0-9a-f]{16}$/.test(shareUrl))

/* A fresh context: no session, no localStorage, nothing the portal put there.
   Sharing that works only for somebody already signed in is not sharing. */
const stranger = await browser.newContext()
const guest = await stranger.newPage()
await guest.goto(shareUrl, { waitUntil: 'domcontentloaded' })
await guest.locator('[data-testid=live-report]').waitFor({ timeout: 20000 })
const shared = await guest.locator('body').innerText()

ok('a stranger with the link reads the report',
  await guest.locator('[data-testid=live-report]').isVisible())
ok('and is told it was shared with them',
  shared.includes('Shared with you'))
/* No rail, no sign-in, no way into the rest of the project. */
ok('and is offered nothing else in the portal',
  !shared.includes('Schedules') && !shared.includes('Settings'))

/* Revoked from the project, and dead for the stranger on the next request —
   not at the next expiry, which is what a signature alone would give. */
const shareId = shareUrl.split('/s/')[1]
await fetch(`${process.env.API}/v1/shares/${shareId}`, {
  method: 'DELETE', headers: { authorization: `Bearer ${process.env.TOKEN}` },
})
await guest.reload({ waitUntil: 'domcontentloaded' })
await guest.locator('text=does not open').waitFor({ timeout: 15000 })
ok('revoking stops it on the next request',
  (await guest.locator('body').innerText()).includes('This link does not open'))
await stranger.close()

/* -- Deleting, and what stops it -----------------------------------------
   The store checks the tenant and nothing else, so both rules live above it:
   who may remove a definition, and whether anything still reads it. */

await page.goto(`${B}/data`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=datasets-card] [data-testid=delete-action]').first()
  .waitFor({ timeout: 15000 })

/* invoices is read by billing-summary. Removing it would leave that report to
   fail on the next open — or at 06:00 on the first, naming a dataset that no
   longer exists to explain itself. */
const invoices = page.locator('[data-testid=datasets-card] li')
  .filter({ hasText: 'invoices' }).first()
await invoices.locator('[data-testid=delete-action]').click()
await invoices.locator('[data-testid=delete-confirm]').click()
await invoices.locator('[data-testid=delete-refused]').waitFor({ timeout: 15000 })
const stopped = await invoices.locator('[data-testid=delete-refused]').innerText()
ok('a dataset a report reads is not deleted', stopped.includes('still read by'))
ok('and the refusal names the report', stopped.includes('billing-summary'))

/* The report still runs, which is the claim the refusal was protecting. */
await page.goto(`${B}/reports/billing-summary`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=live-report]').waitFor({ timeout: 20000 })
ok('and it still runs', (await page.locator('[data-testid=live-report]').innerText())
  .includes('154,651.50'))

/* A viewer's token reached the store directly before this. It no longer does. */
const asViewer = await fetch(`${process.env.API}/v1/definitions/Dataset/customer-list`, {
  method: 'DELETE', headers: { authorization: `Bearer ${process.env.VIEWER}` },
})
ok('a viewer may not delete anything', asViewer.status === 403)

/* -- The connection test -------------------------------------------------
   It used to wait nine hundred milliseconds and report twenty-four tables
   whatever had been typed. A test that cannot fail is worse than no test. */

await page.goto(`${B}/data`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=test-connection]').first().waitFor({ timeout: 15000 })
await page.locator('[data-testid=test-connection]').first().click()
await page.locator('[data-testid=probe-result]').first().waitFor({ timeout: 15000 })
const probe = await page.locator('[data-testid=probe-result]').first().innerText()
ok('a source that is there answers, and says how fast', /Answered in \d+ ms/.test(probe))

/* And one that is not says so in the driver's words. The demo has one real
   source, so this asks the API directly for a name nothing opened. */
const missing = await fetch(`${process.env.API}/v1/datasources/not-a-source/test`, {
  method: 'POST', headers: { authorization: `Bearer ${process.env.TOKEN}` },
}).then((r) => r.json())
ok('a source that is not there does not answer ok', missing.ok === false)
ok('and the failure says which one', String(missing.error).includes('not-a-source'))

/* Testing a connection opens one to somebody's warehouse. Not a reader's
   business, and neither is which sources exist. */
const asReader = await fetch(`${process.env.API}/v1/datasources/warehouse/test`, {
  method: 'POST', headers: { authorization: `Bearer ${process.env.VIEWER}` },
})
ok('a viewer may not test connections', asReader.status === 403)

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
process.exit(fails ? 1 : 0)
