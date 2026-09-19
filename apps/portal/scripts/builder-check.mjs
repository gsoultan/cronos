import { chromium } from 'playwright'
import { until } from './until.mjs'
const B = process.env.BASE ?? 'http://localhost:5173'
const b = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const p = await (await b.newContext({ viewport:{width:1600,height:1050}, deviceScaleFactor:2 })).newPage()
const errs = []
p.on('pageerror', e => errs.push(String(e)))
p.on('console', m => m.type() === 'error' && errs.push(m.text()))
let fails = 0
const ok = (n, c) => { console.log(`  ${c ? 'ok  ' : 'FAIL'} ${n}`); if (!c) fails++ }

await p.goto(B + '/reports/new', { waitUntil: 'domcontentloaded' })
await p.click('input[placeholder="Choose a dataset"]')
await p.locator('[role="option"]:visible').first().click()
await p.waitForSelector('[data-testid=block-palette]')

// One artifact: no Dashboards section, and "dashboard" survives as a template.
ok('no Dashboards nav item',
  await p.locator('nav[aria-label=Main] >> text=Dashboards').count() === 0)
ok('the empty canvas offers starting templates',
  await p.locator('[data-testid=template-dashboard]').isVisible())
await p.click('[data-testid=template-dashboard]')
ok('the Dashboard template lays out blocks',
  await until(() => p.locator('[data-testid=layout-canvas] > div').count(), 6))

// A block may read a different dataset — what replaces a Dashboard kind.
await p.click('[data-testid=layout-canvas] > div >> nth=4')
await p.waitForTimeout(250)
await p.click('[data-testid=inspector] input[value*="report default"]')
await p.locator('[role="option"]:visible', { hasText: 'Shipments' }).first().click()
ok('a block can read a different dataset',
  await until(async () => (await p.locator('[data-testid=layout-canvas] >> text=Shipments').count()) > 0, true))
ok('switching dataset re-seeds the fields rather than blanking them',
  await p.locator('[data-testid=inspector] input[value="Weight (kg)"], [data-testid=inspector] input[value="Cost"]').count() > 0)

// Back to a clean slate for the sizing assertions below.
await p.reload({ waitUntil: 'domcontentloaded' })
await p.click('input[placeholder="Choose a dataset"]')
await p.locator('[role="option"]:visible').first().click()
await p.waitForSelector('[data-testid=block-palette]')

const canvasBefore = await p.locator('[data-testid=inspector]').boundingBox()
ok('inspector shows report settings with nothing selected',
  await p.locator('[data-testid=inspector] h2:has-text("Report")').isVisible())

await p.click('button[aria-label="Add Number"]')
ok('adding a block selects it',
  await until(() => p.locator('[data-testid=inspector] h2:has-text("Block")').isVisible(), true))

for (const b2 of ['Add Bar chart', 'Add Table']) {
  await p.click(`button[aria-label="${b2}"]`); await p.waitForTimeout(200)
}
const canvas = await p.locator('[data-testid=layout-canvas]').boundingBox()
ok(`canvas is large (${Math.round(canvas.width)}×${Math.round(canvas.height)})`,
  canvas.width >= 950 && canvas.height >= 600)

// WYSIWYG: real components, not sketches.
ok('canvas renders a real chart (svg present)',
  await p.locator('[data-testid=layout-canvas] svg').count() > 0)
ok('canvas renders a real table header',
  await p.locator('[data-testid=layout-canvas] >> text=CUSTOMER').count() > 0)

// Width control changes the rendered width.
await p.click('[data-testid=layout-canvas] > div >> nth=1')
const widthOf = async () =>
  Math.round((await p.locator('[data-testid=layout-canvas] > div').nth(1).boundingBox()).width)
const w1 = await widthOf()
await p.click('[role=radio][title="Full"]')
await until(async () => (await widthOf()) !== w1, true)
const w2 = await widthOf()
ok(`width control resizes the block (${Math.round(w1)} → ${Math.round(w2)})`, w2 > w1 * 1.5)

// Delete removes the selection.
const n1 = await p.locator('[data-testid=layout-canvas] > div').count()
await p.keyboard.press('Delete')
ok('Delete removes the selected block',
  await until(() => p.locator('[data-testid=layout-canvas] > div').count(), n1 - 1))
ok('deselecting returns the inspector to report settings',
  await p.locator('[data-testid=inspector] h2:has-text("Report")').isVisible())

/* -- Every chart the palette offers ---------------------------------------- */

/*
 * The canvas draws blocks with the components that will render them, and the
 * ones it has no component for used to draw `null` — an empty cell where the
 * author had just dropped a block. Every palette entry has to put something on
 * the canvas, or the palette is offering blocks that vanish.
 */
const kinds = await p.locator('[data-testid=block-palette] button').evaluateAll(
  (bs) => bs.map((b) => b.getAttribute('aria-label')?.replace(/^Add /, '')))
ok(`the palette offers every chart (${kinds.length})`, kinds.length >= 16)

for (const label of ['Pie chart', 'Funnel', 'Map', 'Treemap', 'Heatmap', 'Gauge', 'Bubble']) {
  const before = await p.locator('[data-testid=layout-canvas] > div').count()
  await p.click(`[data-testid=block-palette] button[aria-label="Add ${label}"]`)
  await until(() => p.locator('[data-testid=layout-canvas] > div').count(), before + 1)

  const cell = p.locator('[data-testid=layout-canvas] > div').nth(before)
  const box = await cell.boundingBox()
  ok(`${label} draws something on the canvas`,
    box !== null && box.height > 40 && (await cell.innerText()).trim().length > 0)
  await p.keyboard.press('Delete')
  await until(() => p.locator('[data-testid=layout-canvas] > div').count(), before)
}

/*
 * The kinds that have no component of their own draw their own shape.
 *
 * These were drawn with the nearest component that existed, which was worse
 * than an empty cell rather than better: a waterfall came out as a plain bar
 * chart, a combo as the same chart without its line, an area as a line with
 * nothing under it, and a gauge as a stat tile adrift in a card three times its
 * height. Each was a confident picture of a chart the report does not draw.
 */
for (const label of ['Bars + line', 'Waterfall', 'Area chart', 'Gauge']) {
  const before = await p.locator('[data-testid=layout-canvas] > div').count()
  await p.click(`[data-testid=block-palette] button[aria-label="Add ${label}"]`)
  await until(() => p.locator('[data-testid=layout-canvas] > div').count(), before + 1)

  const cell = p.locator('[data-testid=layout-canvas] > div').nth(before)
  const text = await cell.innerText()
  ok(`${label} draws its own shape, not the nearest component's`,
    text.includes('Sketch') && (await cell.locator('svg').count()) > 0)
  await p.keyboard.press('Delete')
  await until(() => p.locator('[data-testid=layout-canvas] > div').count(), before)
}

/* -- The report's filters -------------------------------------------------- */

/*
 * These were stored and carried but never editable: a report could be given
 * filters in YAML and then never changed from the builder, and the form could
 * not create one at all.
 */
await p.click('[data-testid=layout-canvas]', { position: { x: 5, y: 5 } })
ok('the inspector offers the report filters',
  await p.locator('[data-testid=report-filters]').isVisible())

await p.click('[data-testid=add-filter]')
await until(() => p.locator('[aria-label="Filter 1 label"]').count(), 1)
await p.fill('[aria-label="Filter 1 label"]', 'Period')
ok('the name follows the label until somebody types one',
  (await p.inputValue('[aria-label="Filter 1 name"]')) === 'period')

// A filter says what it narrows per dataset, because a report's blocks may
// read different ones.
const binds = await p.locator('[data-testid=report-filters] [aria-label^="Filter 1 in"]').count()
ok(`a field picker per dataset the report reads (${binds})`, binds >= 1)

await p.screenshot({ path: `${process.env.SHOT_DIR ?? 'shots'}/19-report-filters.png` })

await p.screenshot({ path: `${process.env.SHOT_DIR ?? 'shots'}/18-report-editor.png` })
console.log(errs.length ? '\nERRORS:\n' + errs.join('\n') : '\nno console errors')
console.log(fails ? `${fails} failed` : 'all passed')
await b.close()
process.exit(fails ? 1 : 0)
