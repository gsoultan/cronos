/* The API name: collapsed by default, follows the name until edited, then stops. */
import { chromium } from 'playwright'
const B = process.env.BASE ?? 'http://localhost:5273'
const b = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const p = await (await b.newContext({ viewport: { width: 1600, height: 1050 } })).newPage()
const errs = []
p.on('pageerror', e => errs.push(String(e)))
let f = 0
const ok = (n, c) => { console.log(`  ${c ? 'ok  ' : 'FAIL'} ${n}`); if (!c) f++ }

await p.goto(B + '/reports/new', { waitUntil: 'networkidle' })
ok('no "Identifier" label anywhere', await p.locator('text=Identifier').count() === 0)
ok('collapsed by default — no input on screen',
  await p.locator('[data-testid=inspector] input[value*="untitled"]').count() === 0)

await p.fill('input[aria-label="Report name"]', 'Monthly invoice statement')
await p.waitForTimeout(300)
ok('follows the name',
  (await p.locator('[data-testid=inspector] code').innerText()).includes('monthly-invoice-statement'))
await p.click('[data-testid=inspector] button:has-text("Change")')
await p.waitForSelector('text=API name')
ok('opening it shows where the name will appear',
  await p.locator('[data-testid=inspector] >> text=…/reports/').isVisible())
ok('expands to an input', await p.locator('[data-testid=inspector] input.font-mono, [data-testid=inspector] input').count() > 0)
ok('states the consequence at the point of editing',
  await p.locator('text=stop resolving').isVisible())

const slug = p.locator('[data-testid=inspector] input').last()
await slug.fill('q3-statement')
await p.fill('input[aria-label="Report name"]', 'Monthly invoice statement v2')
await p.waitForTimeout(300)
ok('a deliberate edit survives a later name change',
  await slug.inputValue() === 'q3-statement')

console.log(errs.length ? '\nERRORS:\n' + errs.join('\n') : '\nno console errors')
console.log(f ? `${f} failed` : 'all passed')
await b.close()
process.exit(f ? 1 : 0)
