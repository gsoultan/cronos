/*
 * The app shell: header, sidebar collapse, persistence, focus mode.
 * Editor-specific behaviour lives in builder-check.mjs.
 */
import { chromium } from 'playwright'

const B = process.env.BASE ?? 'http://localhost:5273'
const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const page = await (await browser.newContext({ viewport: { width: 1600, height: 1000 } })).newPage()

let fails = 0
const ok = (name, cond) => { console.log(`  ${cond ? 'ok  ' : 'FAIL'} ${name}`); if (!cond) fails++ }
const collapsed = () => page.getAttribute('[data-testid=sidebar]', 'data-collapsed')
const mainWidth = async () => (await page.locator('#main').boundingBox()).width

await page.goto(`${B}/data`, { waitUntil: 'networkidle' })
await page.waitForSelector('[data-testid=sidebar]')
ok('sidebar starts expanded', await collapsed() === 'false')
ok('header renders the breadcrumb', await page.locator('nav[aria-label=Breadcrumb]').isVisible())

const wide = await mainWidth()
await page.click('[data-testid=sidebar-toggle]')
await page.waitForTimeout(400)
ok('toggle collapses the sidebar', await collapsed() === 'true')
const narrow = await mainWidth()
ok(`content grows when collapsed (${Math.round(wide)} → ${Math.round(narrow)})`, narrow > wide)
ok('collapsed rail still shows the workspace trigger',
  await page.locator('[data-testid=workspace-trigger]').isVisible())

await page.keyboard.press('[')
await page.waitForTimeout(300)
ok('"[" expands again', await collapsed() === 'false')

// The shortcut must not fire while typing.
await page.click('input[placeholder="Search reports and data…"]').catch(() => {})
await page.locator('header input[type=search]').fill('a[b')
await page.waitForTimeout(200)
ok('"[" typed into a field does not toggle', await collapsed() === 'false')
ok('the bracket reached the input',
  (await page.locator('header input[type=search]').inputValue()).includes('['))

// Collapse persists across a reload.
await page.click('[data-testid=sidebar-toggle]')
await page.waitForTimeout(200)
await page.reload({ waitUntil: 'networkidle' })
ok('collapse survives a reload', await collapsed() === 'true')

// Focus mode: the editor collapses the rail without writing the preference.
await page.click('[data-testid=sidebar-toggle]')
await page.waitForTimeout(300)
ok('expanded again before entering the editor', await collapsed() === 'false')
await page.goto(`${B}/reports/new`, { waitUntil: 'networkidle' })
await page.waitForTimeout(400)
ok('the editor collapses the rail', await collapsed() === 'true')
await page.goto(`${B}/data`, { waitUntil: 'networkidle' })
await page.waitForTimeout(400)
ok('leaving the editor restores the preference', await collapsed() === 'false')

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
process.exit(fails ? 1 : 0)
