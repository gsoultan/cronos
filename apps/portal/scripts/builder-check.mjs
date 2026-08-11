import { chromium } from 'playwright'
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
await p.waitForTimeout(500)
ok('the Dashboard template lays out blocks',
  await p.locator('[data-testid=layout-canvas] > div').count() === 6)

// A block may read a different dataset — what replaces a Dashboard kind.
await p.click('[data-testid=layout-canvas] > div >> nth=4')
await p.waitForTimeout(250)
await p.click('[data-testid=inspector] input[value*="report default"]')
await p.locator('[role="option"]:visible', { hasText: 'Shipments' }).first().click()
await p.waitForTimeout(500)
ok('a block can read a different dataset',
  await p.locator('[data-testid=layout-canvas] >> text=Shipments').count() > 0)
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
await p.waitForTimeout(300)
ok('adding a block selects it',
  await p.locator('[data-testid=inspector] h2:has-text("Block")').isVisible())

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
await p.waitForTimeout(200)
const w1 = (await p.locator('[data-testid=layout-canvas] > div').nth(1).boundingBox()).width
await p.click('[role=radio][title="Full"]')
await p.waitForTimeout(300)
const w2 = (await p.locator('[data-testid=layout-canvas] > div').nth(1).boundingBox()).width
ok(`width control resizes the block (${Math.round(w1)} → ${Math.round(w2)})`, w2 > w1 * 1.5)

// Delete removes the selection.
const n1 = await p.locator('[data-testid=layout-canvas] > div').count()
await p.keyboard.press('Delete')
await p.waitForTimeout(300)
ok('Delete removes the selected block',
  await p.locator('[data-testid=layout-canvas] > div').count() === n1 - 1)
ok('deselecting returns the inspector to report settings',
  await p.locator('[data-testid=inspector] h2:has-text("Report")').isVisible())

await p.screenshot({ path: `${process.env.SHOT_DIR ?? 'shots'}/18-report-editor.png` })
console.log(errs.length ? '\nERRORS:\n' + errs.join('\n') : '\nno console errors')
console.log(fails ? `${fails} failed` : 'all passed')
await b.close()
process.exit(fails ? 1 : 0)
