import { chromium } from 'playwright'
const B = process.env.BASE ?? 'http://localhost:5173'
const b = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const p = await (await b.newContext({ viewport: { width: 1440, height: 900 } })).newPage()
let fails = 0
const check = (name, ok) => { console.log(`  ${ok ? 'ok  ' : 'FAIL'} ${name}`); if (!ok) fails++ }

await p.goto(`${B}/`, { waitUntil: 'domcontentloaded' })

await p.click('[data-testid=workspace-trigger]')
const acme = await p.locator('[data-testid=workspace-item]').allTextContents()
check('org member does not see Executive (no grant)', !acme.some(t => t.includes('Executive')))
check('editor project Finance is listed', acme.some(t => t.includes('Finance')))
check('viewer project Operations is listed', acme.some(t => t.includes('Operations')))

await p.click('[data-testid=workspace-item]:has-text("Operations")')
await p.waitForTimeout(200)
check('viewer sees no "New report" action',
  await p.locator('button:has-text("New report")').count() === 0)

await p.click('[data-testid=workspace-trigger]')
await p.click('[data-testid=workspace-item]:has-text("Northwind")')
await p.waitForTimeout(200)
await p.click('[data-testid=workspace-trigger]')
const nw = await p.locator('[data-testid=workspace-item]').allTextContents()
check('org admin gets project access "via org"', nw.some(t => t.includes('via org')))
await p.keyboard.press('Escape')
await p.waitForTimeout(150)
check('Escape closes the switcher', await p.locator('[data-testid=workspace-menu]').count() === 0)
check('focus returns to the trigger after Escape',
  await p.evaluate(() => document.activeElement?.dataset.testid === 'workspace-trigger'))

console.log(fails ? `\n${fails} failed` : '\nall passed')
await b.close()
process.exit(fails ? 1 : 0)
