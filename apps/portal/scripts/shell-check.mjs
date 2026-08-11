/*
 * The app shell: header, sidebar collapse, persistence, focus mode.
 * Editor-specific behaviour lives in builder-check.mjs.
 */
import { chromium } from 'playwright'
import { until, settle } from './until.mjs'

const B = process.env.BASE ?? 'http://localhost:5173'
const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const page = await (await browser.newContext({ viewport: { width: 1600, height: 1000 } })).newPage()

let fails = 0
const ok = (name, cond) => { console.log(`  ${cond ? 'ok  ' : 'FAIL'} ${name}`); if (!cond) fails++ }
const collapsed = () => page.getAttribute('[data-testid=sidebar]', 'data-collapsed')
const mainWidth = async () => (await page.locator('#main').boundingBox()).width

await page.goto(`${B}/data`, { waitUntil: 'domcontentloaded' })
await page.waitForSelector('[data-testid=sidebar]')
ok('sidebar starts expanded', await collapsed() === 'false')
ok('header renders the breadcrumb', await page.locator('nav[aria-label=Breadcrumb]').isVisible())

const wide = await mainWidth()
await page.click('[data-testid=sidebar-toggle]')
ok('toggle collapses the sidebar', await until(collapsed, 'true'))
const narrow = await mainWidth()
ok(`content grows when collapsed (${Math.round(wide)} → ${Math.round(narrow)})`, narrow > wide)
ok('collapsed rail still shows the workspace trigger',
  await page.locator('[data-testid=workspace-trigger]').isVisible())

await page.keyboard.press('[')
ok('"[" expands again', await until(collapsed, 'false'))

// The shortcut must not fire while typing.
await page.click('input[placeholder="Search reports and data…"]').catch(() => {})
await page.locator('header input[type=search]').fill('a[b')
await settle()
ok('"[" typed into a field does not toggle', await collapsed() === 'false')
ok('the bracket reached the input',
  (await page.locator('header input[type=search]').inputValue()).includes('['))

// Collapse persists across a reload.
await page.click('[data-testid=sidebar-toggle]')
await until(collapsed, 'true')
await page.reload({ waitUntil: 'domcontentloaded' })
ok('collapse survives a reload', await until(collapsed, 'true'))

// Focus mode: the editor collapses the rail without writing the preference.
await page.click('[data-testid=sidebar-toggle]')
ok('expanded again before entering the editor', await until(collapsed, 'false'))
await page.goto(`${B}/reports/new`, { waitUntil: 'domcontentloaded' })
ok('the editor collapses the rail', await until(collapsed, 'true'))
await page.goto(`${B}/data`, { waitUntil: 'domcontentloaded' })
ok('leaving the editor restores the preference', await until(collapsed, 'false'))

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
process.exit(fails ? 1 : 0)
