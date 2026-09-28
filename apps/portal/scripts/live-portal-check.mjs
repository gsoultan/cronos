/*
 * The portal against a real cronosd.
 *
 * Every other suite runs on sample data. That is deliberate — it is what makes
 * the interface workable before a server exists — but it also means none of
 * them would notice if the API contract moved underneath. This one would.
 */
import { readdirSync, readFileSync } from 'node:fs'
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
/* The axis says exactly what the engine wrote.

   It used to say "Jul \u201926" — the portal reformatting a label the server had
   already formatted, which is a second opinion from the side with less
   information. The engine knew the grain; the chart does not. Worse, the
   datum's key was `month`, so a report grouped by anything else went through a
   date formatter too, and three customers came out as three months of 2001.

   The table still uppercases its heading in CSS, so both assertions read
   innerText — the text a person sees rather than the string that was sent. */
ok('the chart axis is the label the engine wrote',
  body.includes('Jul 2026') && !body.includes('2026-07-01'))
ok('table headings come from the dataset labels',
  body.includes('ISSUED') && body.includes('AMOUNT'))

/* The engine formatted these. A table that formats them again meets
   Number("19,800") and prints NaN down the whole column. */
ok('amounts are not reformatted into NaN', !body.includes('NaN'))

/* The fixture's figures must be nowhere near it. */
ok('the sample figures are gone', !body.includes('49.9M'))

/* -- The report format's promise, computed by the server ------------------ */

/* Set the filter first, because the hint is only information once somebody
   has filtered and a number has not moved. Shown unconditionally it was four
   identical captions on an unfiltered screen, naming a filter the page did not
   even offer a control for — so this now drives the control a reader would. */
await page.locator('[data-testid=filter-region]').fill('North')
await page.locator('[data-testid=apply-filters]').click()
await page.locator('[data-testid=unaffected]').first().waitFor({ timeout: 30000 })

ok('a block says the filter that misses it does not apply',
  (await page.locator('[data-testid=unaffected]').first().innerText()).includes('Region'))

/* -- Connected mode is announced as connected ----------------------------- */
ok('no sample-data banner when connected',
  await page.locator('[data-testid=sample-banner]').count() === 0)

/* -- A report is laid out as a report ------------------------------------- */

/* customer-overview rather than billing-summary: its table has three rows, and
   billing-summary's fills the window, so a hole under a short table could not
   show there. Its own page, because two of these need a viewport the rest of
   this file does not expect — and a new context, so the dark theme it turns on
   stays here. */
const layout = await browser.newPage({ viewport: { width: 1440, height: 900 } })
layout.on('pageerror', (e) => errors.push(String(e)))
await layout.goto(`${B}/reports/customer-overview`, { waitUntil: 'domcontentloaded' })
await layout.getByTestId('chart').first().waitFor({ timeout: 20000 })

/* The chart stylesheet is written for a shadow root, so its selectors are bare,
   and adopted into this document they were global and unlayered. `.grid`
   reached the shell's own `<div class="grid ...">`: from the first chart a
   session drew until it ended, the navigation was half the window. Every page
   without a chart on it was laid out correctly, so only one with a chart can
   check it. */
const sidebar = await layout.getByTestId('sidebar').boundingBox()
const main = await layout.getByRole('main').boundingBox()
ok('a chart leaves the navigation its own width',
  sidebar !== null && main !== null && sidebar.width < 1440 / 4 && main.x < 1440 / 4)

/* Its window was 420px whatever came back, so three rows sat on three hundred
   pixels of nothing. The window's first child is the virtualiser's sizer — as
   tall as the rows it holds. */
const hole = await layout.getByTestId('table-rows').first().evaluate(
  (el) => el.clientHeight - el.firstElementChild.getBoundingClientRect().height)
ok('a three-row table has no hole under it', hole < 1)

/* The sheet's own `--cr-surface: #fff` beat the portal's themed one — an
   adopted sheet is unlayered, so it outranked the unlayered theme by coming
   last — and every chart stayed a white card on a dark page. */
await layout.getByRole('button', { name: /dark theme/i }).click()
await layout.waitForTimeout(300)
const card = await layout.getByTestId('chart').first().evaluate(
  (el) => getComputedStyle(el.firstElementChild).backgroundColor)
ok('a chart is not a white card on a dark page', card !== 'rgb(255, 255, 255)')

/* The rows had a horizontal scroller of their own, so on a narrow screen
   scrolling them left the headings behind and every column was labelled with
   its neighbour's name. Driven by the wheel over the rows, because that is the
   gesture that moved the rows alone; and asserted to have moved, or two
   columns that never scrolled would pass as two that scrolled together. */
await layout.setViewportSize({ width: 390, height: 900 })
await layout.waitForTimeout(300)
const heading = layout.getByTestId('table-head').first().getByText(/^customer$/i)
const cell = layout.getByTestId('table-rows').first().getByText('Aurora Freight', { exact: true })
await cell.hover()
const from = (await heading.boundingBox())?.x ?? 0
await layout.mouse.wheel(200, 0)
await layout.waitForTimeout(200)
const headingAt = (await heading.boundingBox())?.x ?? 0
const cellAt = (await cell.boundingBox())?.x ?? 0
ok("a table's headings scroll with its rows",
  headingAt < from && Math.abs(headingAt - cellAt) < 1)
await layout.close()

/* -- The parcel network: maps in the document ---------------------------------
   A map leans on more of the charts' stylesheet than any other chart — a stage
   whose height is an aspect-ratio, tiles laid in pixels under an SVG, buttons
   placed over both — and in the portal that sheet arrives through @scope and
   @layer (see ServerChart) rather than a shadow root. A rule that did not make
   it through reads as a map of height nothing. Tiles are refused here: a check
   is not a reason to ask OpenStreetMap for anything, and the question is
   whether they are laid, not whether a third party answered. */
const maps = await browser.newPage({ viewport: { width: 1440, height: 900 } })
maps.on('pageerror', (e) => errors.push(String(e)))
await maps.route(/tile\.openstreetmap\.org|api\.mapbox\.com|tile\.googleapis\.com/, (r) => r.abort())
await maps.goto(`${B}/reports/parcel-network`, { waitUntil: 'domcontentloaded' })
await maps.locator('[data-testid=live-report] .geo-stage').first().waitFor({ timeout: 20000 })
ok('every map of the parcel network draws in the portal',
  await maps.locator('[data-testid=live-report] .geo-stage').count() === 8)
ok('the deliveries are drawn in H3 cells',
  await maps.getByTestId('chart').filter({ hasText: 'Parcels per H3 cell' }).locator('.hexes path').count() > 5)
/* The zones on time against the target, in two hues either side of it: the
   server classes each side apart and says so, and the map reads the second
   ramp through the first one's names. */
const target = maps.getByTestId('chart').filter({ hasText: 'On time against the 95% target' })
ok('a diverging map is shaded in two hues, cool below the target and warm above',
  await target.locator('.shapes path').evaluateAll((paths) => {
    const hue = (c) => { const [r, , b] = c.match(/\d+/g).map(Number); return b > r ? 'cool' : 'warm' }
    return new Set(paths.map((p) => hue(getComputedStyle(p).fill))).size === 2
  }))
ok('and a map is as tall as its proportions make it',
  ((await maps.locator('.geo-stage').first().boundingBox())?.height ?? 0) > 200)
ok('an OpenStreetMap basemap is laid under the data',
  await maps.locator('.geo-stage').first().locator('.tiles img').count() > 0)
ok('the routes are drawn from their GeoJSON', await maps.locator('.routes path.route').count() === 11)
ok('and a map whose provider has no key here says so rather than drawing a blank',
  (await maps.getByTestId('chart').filter({ hasText: 'Every delivery' }).locator('.credit').innerText())
    .includes('Mapbox is not set up'))
const viewBoxOf = () => maps.locator('svg.geo').first().getAttribute('viewBox')
const fit = await viewBoxOf()
await maps.getByRole('button', { name: 'Zoom in' }).first().click()
await maps.waitForTimeout(200)
ok('a map zooms in the portal as it does in the embed', (await viewBoxOf()) !== fit)
await maps.close()

/* -- A map of more places than a browser holds ------------------------------
   The demo's fleet report: two hundred thousand van positions, gathered into
   cells by the server and painted on a canvas. In the portal a report's maps
   reach the server through the viewer the page hands the chart, so the check
   is that the part in view is asked for and answered — not only that the first
   paint happened. */
const fleet = await browser.newPage({ viewport: { width: 1440, height: 900 } })
fleet.on('pageerror', (e) => errors.push(String(e)))
await fleet.route(/tile\.openstreetmap\.org|api\.mapbox\.com|tile\.googleapis\.com/, (r) => r.abort())
const fleetViews = []
fleet.on('response', (r) => { if (r.url().endsWith('/map')) fleetViews.push(r.status()) })
// Each render of the report, as asked and as answered: what a map sets has to
// reach the server, and the server's numbers are what show it did.
const renders = []
fleet.on('response', async (r) => {
  if (r.request().method() !== 'POST' || !/\/v1\/reports\/fleet$/.test(r.url())) return
  renders.push({ asked: r.request().postDataJSON(), view: await r.json().catch(() => null) })
})
await fleet.goto(`${B}/reports/fleet`, { waitUntil: 'domcontentloaded' })
const heatMap = fleet.getByTestId('chart').filter({ hasText: 'Where the vans are' })
await heatMap.locator('[part=map-canvas]').waitFor({ timeout: 30000 })
ok('a map of 200,000 places draws in the portal, painted rather than an element each',
  await heatMap.locator('[part=map-canvas]').count() === 1
  // The only marks that are elements are the eleven depots drawn over it.
  && await heatMap.locator('[part=marker]').count() === 11)
ok('the depots drawn over it are named under it',
  (await heatMap.locator('[part~=overlay-key]').innerText()).includes('Depots'))
ok('and says how many places it holds',
  (await heatMap.locator('[part=places]').innerText()).includes('200,000 places'))
ok('its clusters count every one of them',
  (await fleet.getByTestId('chart').filter({ hasText: 'Vans by carrier' })
    .getByRole('button', { name: /locations/ }).count()) > 0)
for (let i = 0; i < 60 && fleetViews.length < 2; i++) await fleet.waitForTimeout(100)
ok('each asks the server for the part in view, and is answered',
  fleetViews.length >= 2 && fleetViews.every((s) => s === 200))

/* A map sets the report's filters. A click on a depot narrows the report to
   its vans, while the depots map stays whole with the one picked marked, so
   the next can be clicked; the filter bar shows what the map set, and using
   the bar for something else does not undo it. */
const rendered = async (seen) => {
  for (let i = 0; i < 150 && renders.length === seen; i++) await fleet.waitForTimeout(100)
  await fleet.waitForTimeout(300)
  return renders.at(-1)
}
const positions = (r) => r?.view?.blocks?.find((b) => b.title === 'Positions')?.value
const whole = positions(renders.at(-1))
const depotsMap = fleet.getByTestId('chart').filter({ has: fleet.locator('h3', { hasText: /^Depots$/ }) })
const rotterdam = depotsMap.getByRole('button', { name: /^Rotterdam,/ })
ok('a depot on the map is a button that filters the report', await rotterdam.count() === 1)
let at = renders.length
await rotterdam.dispatchEvent('click')
const picked = await rendered(at)
ok('a click on a depot narrows the report to its vans',
  JSON.stringify(picked?.asked?.filters?.depot) === '{"op":"in","values":["Rotterdam"]}'
  && !!positions(picked) && positions(picked) !== whole)
ok('the depots map stays whole, with the one picked marked',
  await depotsMap.getByRole('button', { name: /click to/ }).count() === 11
  && await depotsMap.locator('[aria-pressed=true]').getAttribute('aria-label')
    .then((l) => l?.startsWith('Rotterdam,')))
ok('and the filter bar shows the depot the map set',
  await fleet.getByTestId('filter-depot').inputValue() === 'Rotterdam')

at = renders.length
await heatMap.getByRole('button', { name: 'Filter to this view' }).click()
const boxed = await rendered(at)
ok('"Filter to this view" narrows the report to the part of the world in view',
  boxed?.asked?.filters?.where?.op === 'within' && boxed.asked.filters.depot !== undefined)
ok('and the filter bar says which part, in degrees',
  /°N to .*°N, .*°E to .*°E/.test(await fleet.getByTestId('filter-where').innerText()))

at = renders.length
await fleet.getByTestId('report-filters').getByText('Northline', { exact: true }).click()
await fleet.getByTestId('apply-filters').click()
const both = await rendered(at)
ok('applying the bar keeps what the map set, operator and all',
  JSON.stringify(both?.asked?.filters?.depot) === '{"op":"in","values":["Rotterdam"]}'
  && both?.asked?.filters?.where?.op === 'within' && both?.asked?.filters?.carrier !== undefined)

at = renders.length
await fleet.getByTestId('filter-where-clear').click()
const unboxed = await rendered(at)
ok('"Show everywhere" lets the area go, and nothing else',
  unboxed?.asked?.filters?.where === undefined && unboxed?.asked?.filters?.depot !== undefined)
await fleet.close()

/* -- The catalogue: what the project contains ----------------------------- */
await page.goto(`${B}/data`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=datasets-card]').waitFor({ timeout: 15000 })
const data = await page.locator('body').innerText()

/* By the name the author gave them, not the identifier. A definition may carry
   a title, and the whole point of one is that the interface uses it — a page
   headed "statement-lines" is showing somebody a primary key. */
/* Paged, six to a page: the demo holds ten since it gained a parcel network,
   and "Statement lines" is on the second. Paging to it is the claim — every
   dataset the server has is reachable — where asserting it on the first page
   was a claim about how many the demo happened to hold. */
const datasets = page.locator('[data-testid=datasets-card]')
await datasets.getByRole('button', { name: 'Next' }).click()
const second = await datasets.innerText()
ok('the data page lists the datasets the server has',
  data.includes('Invoices') && data.includes('Active customers')
  && second.includes('Statement lines'))
ok('and the source they read', data.includes('warehouse'))
/* The limits are what nobody opens the file for: what one query may spend, and
   how much it may hand back. */
ok('a source shows its limits', data.includes('30s') && data.includes('100,000'))
/* Whether an embedded end customer sees only their own rows. */
ok('a row-scoped dataset says so', data.includes('row scoped'))
/* And the sample fixture is nowhere near it. */
ok('the sample sources are gone', !data.includes('Northwind'))

/* -- Connecting a second source ------------------------------------------
   A connected deployment had no way to add one. The wizard existed, published
   correctly and was reachable only from the sample page, so a project could
   edit and delete the sources it booted with and never gain another: the
   second warehouse was a YAML file and a deploy.

   Opening it is half the assertion. The hooks below the live branch's early
   return meant that clicking this would have thrown "rendered more hooks than
   during the previous render" — a crash that could not happen while nothing
   linked to the panel. */
ok('a connected editor can connect a source',
  await page.locator('[data-testid=connect-source]').count() === 1)
await page.click('[data-testid=connect-source]')
await page.locator('text=What are you connecting?').waitFor({ timeout: 15000 })
ok('and the wizard opens on the connected page',
  (await page.locator('body').innerText()).includes('Choose a source'))
ok('without throwing', errors.length === 0)
if (errors.length) console.log(errors.slice(0, 3).map((e) => `       ${e}`).join('\n'))

/* -- The switcher shows this deployment, not the fixture ------------------
   It listed two organisations nobody belongs to — Acme Logistics and
   Northwind Trading, out of the sample directory — and clicking a project in
   them changed nothing, because the project is in the token and only the
   server can mint one. /v1/auth/project has answered both halves the whole
   time and nothing called it. */
await page.click('[data-testid=workspace-trigger]')
await page.locator('[data-testid=workspace-menu]').waitFor({ timeout: 15000 })
await page.locator('[data-testid=workspace-menu] [role=menuitem]').first()
  .waitFor({ timeout: 15000 })
await page.waitForTimeout(600)   // the list is fetched when the menu opens
const menu = await page.locator('[data-testid=workspace-menu]').innerText()
ok('the switcher names the project this session is in', menu.includes('finance'))
ok('and offers exactly one organisation, the one the account is in',
  !menu.includes('Northwind'))
ok('and no project out of the sample directory', !menu.includes('Operations'))
ok('and it could read the list',
  await page.locator('[data-testid=workspace-error]').count() === 0)
await page.keyboard.press('Escape')

await page.goto(`${B}/schedules`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=schedules-card]').waitFor({ timeout: 15000 })
const schedules = await page.locator('body').innerText()
ok('the schedules page lists the real schedule', schedules.includes('Monthly statements'))
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

/* -- Parameters ----------------------------------------------------------
   Carried through an edit before they were modelled, so a parameterised
   dataset survived being opened and saved — and could only be created by
   editing the file, which made the portal a second-class way to author
   exactly the datasets that need the most care. */

await page.goto(`${B}/data/datasets/invoices/edit`, { waitUntil: 'domcontentloaded' })
await page.locator('button:has-text("Parameters")').waitFor({ timeout: 15000 })
ok('a parameterised dataset says how many it has',
  (await page.locator('button:has-text("Parameters")').innerText()).match(/\d/) !== null)

await page.locator('button:has-text("Parameters")').click()
await page.locator('[data-testid=params-editor]').waitFor({ timeout: 15000 })
const declared = await page.locator('[data-testid=param-name]')
  .evaluateAll((n) => n.map((e) => e.value))
ok('and shows the ones the file declares', declared.includes('from'))

/* A default the file carries, which is the part that decides whether a report
   may omit the parameter at all. */
const defaults = await page.locator('[data-testid=param-default]')
  .evaluateAll((n) => n.map((e) => e.value))
ok('with the defaults that make them optional', defaults.some((v) => v.includes('2020-01-01')))

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

/* fleet draws its depots over its vans and has filters a map sets. The builder
   rewrites a map's block and every filter wholesale, so a key it does not read
   is a key a save deletes — the overlays, a filter's control, an area's pair of
   fields — and opening the report would have warned that saving drops them. */
await page.goto(`${B}/reports/fleet/edit`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=canvas-block]').first().waitFor({ timeout: 20000 })
ok('a report whose maps filter and overlay is not beyond the editor',
  await page.locator('[data-testid=unmodelled-warning]').count() === 0)
ok('and an area filter is bound through a latitude and a longitude',
  await page.getByLabel(/, latitude$/).count() >= 1
  && (await page.getByLabel(/, latitude$/).first().inputValue()) !== ''
  && (await page.getByLabel(/, longitude$/).first().inputValue()) !== '')
await page.locator('button:has-text("Save report")').click()
await page.waitForURL(/\/$/, { timeout: 15000 })
const fleetSaved = await fetch(`${process.env.API}/v1/definitions/Report/fleet`, {
  headers: { authorization: `Bearer ${process.env.TOKEN}` },
}).then((r) => r.text())
ok('a save keeps the datasets drawn over a map', fleetSaved.includes('overlays:'))
ok('and the control a filter was given', fleetSaved.includes('control: checkboxes'))
ok('and the pair of fields an area narrows', /pings: "?lat,lon"?/.test(fleetSaved))

/* -- Activity: what ran, and who got it ----------------------------------
   Nothing has run yet, and the page says so rather than showing an empty
   table that could equally mean a broken query. */

await page.goto(`${B}/activity`, { waitUntil: 'domcontentloaded' })
/* Two waits, and both are load-bearing.

   The first says React mounted. `text=Activity` matches the nav rail entry and
   the breadcrumb, which render with the shell — it proves the app is on screen
   and nothing about whether the run history arrived.

   The second says the query settled. ActivityPage returns a Loading… element
   until it does, so waiting only for the word read the body in between and
   asserted an empty state against the text "Loading…" — passing whenever the
   request happened to be quick. That is the flake this came from.

   Waiting only for the loader to detach fails the other way, and worse: at
   domcontentloaded React has not mounted, so the element is not there yet,
   `detached` is satisfied at once, and the body being read is an empty shell.
   That was tried here and failed every run rather than some of them.

   Order matters. Once the nav is on screen the route has rendered in the same
   commit, so the loader is present if the query is pending and already gone if
   it is not — and detached resolves immediately in that second case. */
await page.locator('text=Activity').first().waitFor({ timeout: 15000 })
await page.locator('[data-testid=runs-loading]').waitFor({ state: 'detached', timeout: 15000 })
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

/* -- The audit trail -----------------------------------------------------
   A product whose claim is governed access to somebody else's customers' data
   has to be able to say who read what. The seam had a discarding default and
   nothing called it, which is a compliance answer of "we could". */

const trail = readFileSync('/tmp/cronosd-portal.log', 'utf8')
  .split('\n').filter((l) => l.includes('msg=audit'))

ok('reads are recorded', trail.some((l) => l.includes('action=report.read')))
ok('and so is publishing', trail.some((l) => l.includes('action=definition.publish')))
/* The half a successes-only log cannot show. */
ok('and what was refused', trail.some((l) => l.includes('result=refused')))
ok('sharing is recorded', trail.some((l) => l.includes('action=share.create')))
/* Anonymous by design, and the entry says so rather than inventing a person. */
ok('including an anonymous open', trail.some((l) => l.includes('action=share.open')))
/* Every entry carries the id of the request that caused it, so the audit and
   the request log are one story rather than two accounts of it. */
ok('every entry joins the request that caused it',
  trail.length > 0 && trail.every((l) => l.includes('detail.request=')))

/* -- Sending ------------------------------------------------------------
   The panel has offered to email a report since it was drawn and there was
   nothing behind it. The channels have existed all along — schedules deliver
   through them every month — so what was missing was the decision to send one
   now, to addresses somebody typed. */

/* The format offered is one this report has. It used to offer PDF, Excel and
   CSV for every report, so an interactive-only one produced a refusal from the
   server after somebody had typed the recipients — which is where this was
   found. */
await page.goto(`${B}/reports/customer-overview`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=share-button]').waitFor({ timeout: 20000 })
await page.locator('[data-testid=share-button]').click()
/* This deployment has a file channel and no mail relay, and the panel offers
   what exists: it used to offer email and Telegram whatever was configured, so
   a deployment with neither showed two options that could only fail — after
   somebody had typed eight addresses into one of them. */
ok('a channel this deployment has is offered',
  await page.locator('[data-testid=channel-file]').count() === 1)
ok('and one it does not have is not',
  await page.locator('[data-testid=channel-email]').count() === 0)

await page.locator('[data-testid=channel-file]').click()
await page.locator('[data-testid=share-panel] textarea').first().fill('to-a-colleague')
await page.locator('[data-testid=send-now]').click()
await page.locator('[data-testid=send-result]').waitFor({ timeout: 30000 })
ok('sending says how many it reached',
  (await page.locator('[data-testid=send-result]').innerText()).includes('1 recipient'))

/* And a document actually landed. "Sent" that produced no file is the state
   this tab was in for its whole existence. */
const delivered = readdirSync(process.env.DELIVERIES ?? '/tmp', { recursive: true })
  .filter((f) => String(f).includes('to-a-colleague'))
ok('and a document was delivered', delivered.length > 0)

/* -- Secrets -------------------------------------------------------------
   A map drawn with no basemap and a warehouse that will not open are both a
   secret nobody set. Until Settings had a Secrets tab the only place that said
   so was the server's log, and the only fix was an environment variable and a
   restart. live-portal.sh exports a CRONOS_SECRETS_KEY, so this server can
   keep one the way a deployment set up through first run can.

   Last among the browser sections, because it changes what parcel-network
   draws: the maps section above asserts the note a keyless Mapbox map shows. */

const API = process.env.API
const bearer = (token) => ({ authorization: `Bearer ${token}` })
const secretsPage = await browser.newPage({ viewport: { width: 1440, height: 900 } })
secretsPage.on('pageerror', (e) => errors.push(String(e)))
await secretsPage.goto(`${B}/settings`, { waitUntil: 'domcontentloaded' })
await secretsPage.locator('[role=tab]:has-text("Secrets")').click()
await secretsPage.locator('[data-testid=secrets-panel]').waitFor({ timeout: 15000 })

/* Listed although nothing stores it and no report names it: a Mapbox map with
   no key of its own reads the provider's default, and that is exactly the name
   somebody fixing the map has to be told. */
const mapboxRow = secretsPage.locator('[data-testid=secret-row][data-secret="mapbox-token"]')
ok('Settings lists the key a Mapbox map reads, though no file names it',
  await mapboxRow.count() === 1)
ok('and says nothing answers it',
  (await mapboxRow.getByTestId('secret-source').innerText()) === 'Missing')
ok('and which report is missing it',
  (await mapboxRow.getByTestId('secret-used-by').innerText()).includes('parcel-network'))

/* Set the way somebody fixing the map would. Any pk. value does: the tiles are
   refused below, and the claim is where they are asked for, not whether Mapbox
   answers a made-up token. */
const FAKE_TOKEN = 'pk.live-portal-check'
await mapboxRow.getByTestId('secret-set').click()
await mapboxRow.getByTestId('secret-value').fill(FAKE_TOKEN)
await mapboxRow.getByTestId('secret-save').click()
await mapboxRow.getByTestId('secret-source').filter({ hasText: 'Stored in this project' })
  .waitFor({ timeout: 15000 })
ok('setting it through the tab stores it in the project',
  (await mapboxRow.getByTestId('secret-source').innerText()) === 'Stored in this project')
ok('and says the report reads it without a restart',
  (await secretsPage.getByTestId('secrets-said').innerText()).includes('nothing needs restarting'))
/* Write-only means nowhere: not in the page's text and not in a field left
   holding it after the save. */
const fields = await secretsPage.locator('input').evaluateAll((n) => n.map((e) => e.value))
ok('and nothing on the page holds the value afterwards',
  !(await secretsPage.locator('body').innerText()).includes(FAKE_TOKEN)
  && !fields.some((v) => v.includes(FAKE_TOKEN)))

/* The server agrees, and never hands the value back — to the editor who set it
   any more than to anybody else. */
const listedText = await fetch(`${API}/v1/secrets`, { headers: bearer(process.env.TOKEN) })
  .then((r) => r.text())
const listedMapbox = JSON.parse(listedText).secrets.find((s) => s.name === 'mapbox-token')
ok('the server lists it as stored here', listedMapbox?.source === 'project')
ok('and the list carries no value', !listedText.includes(FAKE_TOKEN))

/* The report draws with it on its next render — no restart, no redeploy. A new
   page, so nothing the first visit cached can stand in for the server's answer;
   tiles recorded and refused, for the reason the maps section gives. */
const withKey = await browser.newPage({ viewport: { width: 1440, height: 900 } })
withKey.on('pageerror', (e) => errors.push(String(e)))
const tilesAsked = []
await withKey.route(/tile\.openstreetmap\.org|api\.mapbox\.com|tile\.googleapis\.com/, (r) => {
  tilesAsked.push(r.request().url())
  return r.abort()
})
await withKey.goto(`${B}/reports/parcel-network`, { waitUntil: 'domcontentloaded' })
await withKey.locator('[data-testid=live-report] .geo-stage').first().waitFor({ timeout: 20000 })
const everyDelivery = withKey.getByTestId('chart').filter({ hasText: 'Every delivery' })
await everyDelivery.locator('.tiles img').first().waitFor({ state: 'attached', timeout: 15000 })
const tileSources = await everyDelivery.locator('.tiles img')
  .evaluateAll((n) => n.map((e) => e.getAttribute('src') ?? ''))
ok('a Mapbox map now lays tiles from api.mapbox.com',
  tileSources.length > 0 && tileSources.every((s) => s.startsWith('https://api.mapbox.com/')))
ok('with the token that was set', tileSources.some((s) => s.includes(`access_token=${FAKE_TOKEN}`)))
ok('and the browser asked Mapbox for them',
  tilesAsked.some((u) => u.startsWith('https://api.mapbox.com/')))
ok('and the note that Mapbox is not set up is gone',
  !(await withKey.getByTestId('live-report').innerText()).includes('Mapbox is not set up'))
await withKey.close()

/* A viewer reads reports and nothing that opens anything: which secrets a
   project holds, and what each one opens, is a map of where to aim. */
const viewerList = await fetch(`${API}/v1/secrets`, { headers: bearer(process.env.VIEWER) })
ok('a viewer may not read the list of secrets', viewerList.status === 403)
const viewerSet = await fetch(`${API}/v1/secrets/mapbox-token`, {
  method: 'PUT', headers: { ...bearer(process.env.VIEWER), 'content-type': 'application/json' },
  body: JSON.stringify({ value: 'pk.from-a-viewer' }),
})
ok('nor set one', viewerSet.status === 403)

/* And the tab is not offered to one. The token baked into this portal is an
   editor's, so a viewer's session is written where the portal reads who is
   signed in — which is all the tab is decided by. The 403s above are what
   actually stop a viewer; this is the interface not offering a door that
   opens onto one. */
const viewerContext = await browser.newContext()
await viewerContext.addInitScript(() => localStorage.setItem('cronos.user', JSON.stringify({
  id: 'sam', email: 'sam@acme.example', org: 'acme', project: 'finance', role: 'viewer',
})))
const viewerPage = await viewerContext.newPage()
viewerPage.on('pageerror', (e) => errors.push(String(e)))
await viewerPage.goto(`${B}/settings`, { waitUntil: 'domcontentloaded' })
await viewerPage.locator('[role=tab]:has-text("Security")').waitFor({ timeout: 15000 })
ok('and a viewer is not offered the Secrets tab',
  await viewerPage.locator('[role=tab]:has-text("Secrets")').count() === 0)
await viewerContext.close()

/* Removing it says what uses it and both ways that can go, then does it. The
   deployment has no Mapbox token of its own here, so the map loses its key. */
await mapboxRow.getByTestId('secret-remove').click()
const asked = await mapboxRow.getByTestId('secret-remove-ask').innerText()
ok('removing one names what uses it and what happens to it',
  asked.includes('parcel-network') && asked.includes('without a basemap'))
await mapboxRow.getByTestId('secret-remove-confirm').click()
await mapboxRow.getByTestId('secret-source').filter({ hasText: 'Missing' })
  .waitFor({ timeout: 15000 })
ok('and once removed, nothing answers it again',
  (await mapboxRow.getByTestId('secret-source').innerText()) === 'Missing')
ok('which the tab says in so many words',
  (await secretsPage.getByTestId('secrets-said').innerText()).includes('Nothing answers it now'))
await secretsPage.close()

/* The same key from the builder, where the map is being drawn. A map block
   says whether its basemap's key is set and takes one in place — inside the
   report's own form, so Enter in the key field must store the key and not
   publish the report around it. */
const builder = await browser.newPage({ viewport: { width: 1440, height: 900 } })
builder.on('pageerror', (e) => errors.push(String(e)))
await builder.goto(`${B}/reports/parcel-network/edit`, { waitUntil: 'domcontentloaded' })
await builder.locator('[data-testid=canvas-block]').first().waitFor({ timeout: 20000 })
await builder.locator('[data-testid=canvas-block]').filter({ hasText: 'Every delivery' }).first().click()
const keyPanel = builder.locator('[data-testid=basemap-key][data-secret="mapbox-token"]')
await keyPanel.waitFor({ timeout: 10000 })
ok('the builder says a Mapbox map’s key is missing',
  (await keyPanel.getByTestId('secret-source').innerText()) === 'Missing')

/* A secret token can change the account, and a map key is sent to every
   reader, so one is refused before it is stored anywhere. */
await keyPanel.getByTestId('basemap-key-set').click()
await keyPanel.getByTestId('secret-value').fill('sk.not-for-a-browser')
await keyPanel.getByTestId('secret-save').click()
await keyPanel.getByTestId('secret-error').waitFor({ timeout: 5000 }).catch(() => {})
ok('and refuses a Mapbox secret token as the key',
  await keyPanel.getByTestId('secret-error').count() === 1
  && (await keyPanel.getByTestId('secret-error').innerText()).includes('pk.'))

await keyPanel.getByTestId('secret-value').fill(FAKE_TOKEN)
await keyPanel.getByTestId('secret-value').press('Enter')
await keyPanel.getByTestId('secret-source').filter({ hasText: 'Stored in this project' })
  .waitFor({ timeout: 15000 }).catch(() => {})
const stayed = builder.url().endsWith('/reports/parcel-network/edit')
ok('without publishing the report or leaving it', stayed)
ok('and stores a public one from the builder', stayed
  && (await keyPanel.getByTestId('secret-source').innerText()) === 'Stored in this project')
await builder.close()

/* Back where it started, for whatever reads the list after this. */
const unset = await fetch(`${API}/v1/secrets/mapbox-token`, {
  method: 'DELETE', headers: bearer(process.env.TOKEN),
})
ok('and a key set from the builder is the same secret the tab removes', unset.status === 204)

/* -- The wizard keeps the password it asks for ----------------------------
   It used to collect one and throw it away, while the definition it published
   named ${secret:<name>_password} — so every database connected through the
   portal had a connection string that could not open. Nothing listens on port
   1, so the connection itself fails; the claim is that it fails for that
   reason and not for want of a password. The password has a space in it,
   because it sits in a URL and a space is the first thing that breaks one —
   stored as typed, and encoded by the server where the definition says
   `|url`. */

await page.goto(`${B}/data`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=connect-source]').click()
await page.locator('text=What are you connecting?').waitFor({ timeout: 15000 })
await page.locator('button:has-text("PostgreSQL")').click()
await page.locator('button:has-text("Continue")').click()
await page.locator('[data-testid=source-host]').fill('127.0.0.1')
await page.locator('[data-testid=source-port]').fill('1')
await page.locator('[data-testid=source-database]').fill('scratch')
await page.locator('[data-testid=source-user]').fill('reader')
await page.locator('[data-testid=source-password]').fill('correct horse battery staple')
await page.locator('button:has-text("Continue")').click()
await page.locator('button:has-text("Continue")').click()
await page.locator('[data-testid=source-name]').fill('Scratch warehouse')
/* Waited for and then counted, so a wizard that stores nothing fails the
   assertions below by name rather than stopping the run at a timeout. */
const storedAs = page.locator('[data-testid=stored-as]')
await storedAs.waitFor({ timeout: 5000 }).catch(() => {})
ok('the wizard says which secret the password becomes',
  await storedAs.count() === 1 && (await storedAs.innerText()).includes('scratch-warehouse_password'))
await page.locator('button:has-text("Save source")').click()
await page.locator('[data-testid=sources-card]').filter({ hasText: 'Scratch warehouse' })
  .waitFor({ timeout: 15000 })

const scratchDefinition = await fetch(`${API}/v1/definitions/DataSource/scratch-warehouse`,
  { headers: bearer(process.env.TOKEN) }).then((r) => r.text())
ok('the definition names the secret, encoded for the URL it sits in, and holds no password',
  scratchDefinition.includes('${secret:scratch-warehouse_password|url}')
  && !scratchDefinition.includes('horse'))

await page.goto(`${B}/settings`, { waitUntil: 'domcontentloaded' })
await page.locator('[role=tab]:has-text("Secrets")').click()
const scratchRow = page.locator('[data-testid=secret-row][data-secret="scratch-warehouse_password"]')
await scratchRow.waitFor({ timeout: 15000 })
ok('and the password is stored in the project',
  (await scratchRow.getByTestId('secret-source').innerText()) === 'Stored in this project')
ok('used by the source it was typed for',
  (await scratchRow.getByTestId('secret-used-by').innerText()).includes('scratch-warehouse'))

const scratchProbe = await fetch(`${API}/v1/datasources/scratch-warehouse/test`, {
  method: 'POST', headers: bearer(process.env.TOKEN),
}).then((r) => r.json())
ok('and the source opens with it — what fails is the port, not a missing password',
  !String(scratchProbe.error).includes('not resolved')
  && String(scratchProbe.error).includes('127.0.0.1'))

/* Gone again, so nothing after this — or a rerun against the same store —
   meets a source that cannot connect. */
const dropSource = await fetch(`${API}/v1/definitions/DataSource/scratch-warehouse`, {
  method: 'DELETE', headers: bearer(process.env.TOKEN),
})
const dropSecret = await fetch(`${API}/v1/secrets/scratch-warehouse_password`, {
  method: 'DELETE', headers: bearer(process.env.TOKEN),
})
ok('and what the check made is removed', dropSource.status === 204 && dropSecret.status === 204)

ok(`nothing was thrown in the secrets sections (${errors.length})`, errors.length === 0)

console.log(fails ? `\n${fails} failed` : '\nall passed')

/*
 * And what every page says when the server is not there.
 *
 * A failed query and an empty project arrive at these pages identically — as
 * an empty list — so the Reports page told somebody "No reports in finance
 * yet" whenever the API was unreachable, and offered to help them build their
 * first one. On the page they had opened to check on their reports, during a
 * deploy. Schedules said nothing was scheduled, on the page somebody opens to
 * find out whether tomorrow's statements will go.
 *
 * Only reproducible against a real server: sample mode never queries, so no
 * suite that runs on fixtures can see it, and a unit test would have to invent
 * the failure it is checking for.
 */
await page.route('**/v1/**', (route) => route.abort('connectionrefused'))

for (const [path, name] of [
  ['/', 'reports'],
  ['/data', 'data'],
  ['/schedules', 'schedules'],
  ['/activity', 'activity'],
]) {
  await page.goto(`${B}${path}`, { waitUntil: 'domcontentloaded' })
  // Long enough for the query to fail and retry once.
  await page.waitForTimeout(3000)
  const text = (await page.locator('body').innerText()).replace(/\s+/g, ' ')

  ok(`${name} says it could not read rather than that there is nothing`,
    /could not read/i.test(text))
  ok(`${name} does not claim the project is empty`,
    !/No reports in|Nothing is scheduled|build your first/i.test(text))
}


/*
 * Two editors, and the second is told rather than winning silently.
 *
 * Two people editing one report is a Monday, and until now the second save
 * discarded the first without a word: the author saw their change land and the
 * other found theirs gone the next time they opened the page. The version
 * history means the lost work is recoverable, which sounds like a mitigation
 * and is not — nobody knows to look, because nothing said anything.
 *
 * Driven through the API rather than the browser because what is under test is
 * the contract: an ETag on the read, an If-Match on the write, and a 409 that
 * says what happened. The editors send it; this proves the server enforces it.
 */
const api = process.env.API ?? 'http://localhost:8081'
const token = process.env.TOKEN ?? ''

if (token) {
  const read = await fetch(`${api}/v1/definitions/Report/billing-summary`, {
    headers: { authorization: `Bearer ${token}` },
  })
  const yaml = await read.text()
  const version = (read.headers.get('ETag') ?? '').replace(/"/g, '')

  ok('reading a definition says what version it is at', /^sha256:[0-9a-f]{12}$/.test(version))

  const save = (body, expect) =>
    fetch(`${api}/v1/definitions`, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${token}`,
        'content-type': 'application/yaml',
        ...(expect ? { 'if-match': `"${expect}"` } : {}),
      },
      body,
    })

  // One of them saves.
  const mine = yaml.replace('title: Billing summary', 'title: Billing summary, mine')
  const first = await save(mine, version)
  ok('the first editor saves', first.status === 200)

  // The other saves what they were working on, from the version that is gone.
  const theirs = yaml.replace('title: Billing summary', 'title: Billing summary, theirs')
  const second = await save(theirs, version)
  ok('the second is refused rather than overwriting', second.status === 409)

  const said = (await second.json()).error ?? ''
  ok('and is told somebody else saved it', /somebody else saved it/.test(said))
  ok('and which version it is at now', /sha256:/.test(said))

  // The deployment pipeline's case: no expectation, stored unconditionally.
  const pipeline = await save(theirs)
  ok('a publish with no expectation still overwrites', pipeline.status === 200)
}

await browser.close()
process.exit(fails ? 1 : 0)
