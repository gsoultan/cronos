/* One-screen smoke shot, for checking a styling change quickly. */
import { chromium } from 'playwright'
const BASE = process.env.BASE ?? 'http://localhost:5273'
const PATH = process.argv[2] ?? '/'
const OUT = process.argv[3] ?? 'shots/quick.png'
const COLLAPSE = process.argv[4] === 'collapse'

const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const page = await (await browser.newContext({
  viewport: { width: 1600, height: 1050 }, deviceScaleFactor: 2,
})).newPage()
const errors = []
page.on('pageerror', (e) => errors.push(String(e)))
page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))

await page.goto(BASE + PATH, { waitUntil: 'networkidle' })
if (PATH === '/reports/new') {
  await page.click('input[placeholder="Choose a dataset"]')
  await page.locator('[role="option"]:visible').first().click()
  await page.waitForSelector('[data-testid=builder-palette]')
  for (const b of ['Number', 'Bar chart', 'Line chart', 'Table']) {
    await page.click(`button:has-text("${b}")`)
  }
}
if (COLLAPSE) { await page.click('[data-testid=sidebar-toggle]'); await page.waitForTimeout(400) }
await page.waitForTimeout(500)
await page.screenshot({ path: OUT, fullPage: false })
console.log(errors.length ? `errors:\n${errors.join('\n')}` : 'no console errors')
await browser.close()
