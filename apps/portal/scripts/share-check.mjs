/* Sharing: channels, recipient validation, and what each option discloses. */
import { chromium } from 'playwright'
const B = process.env.BASE ?? 'http://localhost:5173'
const b = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const p = await (await b.newContext({ viewport: { width: 1600, height: 1100 } })).newPage()
const errs = []
p.on('pageerror', e => errs.push(String(e)))
p.on('console', m => m.type() === 'error' && errs.push(m.text()))
let f = 0
const ok = (n, c) => { console.log(`  ${c ? 'ok  ' : 'FAIL'} ${n}`); if (!c) f++ }

await p.goto(B + '/reports/monthly-invoice-statement', { waitUntil: 'domcontentloaded' })
await p.waitForSelector('[data-testid=share-button]')
await p.click('[data-testid=share-button]')
await p.waitForSelector('[data-testid=share-panel]')
ok('share opens from the report', await p.locator('[data-testid=share-panel]').isVisible())
ok('both channels are offered',
  await p.locator('[data-testid=channel-email]').isVisible() &&
  await p.locator('[data-testid=channel-telegram]').isVisible())

// Sending states plainly whose rows go out.
ok('a send is described as a snapshot of your rows',
  await p.locator('text=Recipients see your rows, not their own').isVisible())

// Recipients are validated, and the bad ones are named.
const send = p.locator('[data-testid=send-now]')
ok('cannot send with no recipients', await send.isDisabled())
await p.fill('textarea[aria-label="Email recipients"]', 'dewi@acme.com, not-an-address')
await p.waitForTimeout(250)
ok('a malformed address blocks the send', await send.isDisabled())
ok('and is named rather than just rejected',
  await p.locator('text=Not an address: not-an-address').isVisible())
await p.fill('textarea[aria-label="Email recipients"]', 'dewi@acme.com, finance@acme.com')
await p.waitForTimeout(250)
ok('two valid addresses enable a plural send',
  (await send.innerText()).includes('2 recipients') && await send.isEnabled())

// Telegram explains the constraint rather than presenting an empty box.
await p.click('[data-testid=channel-telegram]')
await p.waitForTimeout(250)
ok('telegram explains the bot must already be in the chat',
  await p.locator('text=/bot .*added to|not let a bot/i').first().isVisible())
ok('telegram size limit is stated', await p.locator('text=50 MB').isVisible())

// Links: the two audiences disclose different things, and say so.
await p.click('[role=tab]:has-text("Get a link")')
await p.waitForSelector('[data-testid=audience-project]')
ok('a project link is marked live',
  (await p.locator('[data-testid=audience-project]').innerText()).includes('live'))
ok('a public link is marked snapshot',
  (await p.locator('[data-testid=audience-anyone]').innerText()).includes('snapshot'))
ok('the project link warns they may see less',
  (await p.locator('[data-testid=audience-project]').innerText())
    .includes('They may see less than you do'))

await p.click('[data-testid=audience-anyone]')
await p.click('input[aria-label="Link expires"]')
await p.locator('[role="option"]:visible', { hasText: 'Never' }).first().click()
await p.waitForTimeout(300)
ok('a never-expiring public link is called out',
  await p.locator('text=password that never rotates').isVisible())

// Channels are configurable, not assumed.
await p.goto(B + '/settings', { waitUntil: 'domcontentloaded' })
await p.click('[role=tab]:has-text("Channels")')
await p.waitForSelector('[data-testid=channel-panel-telegram]')
ok('telegram configuration lists the connected chats',
  await p.locator('[data-testid=channel-panel-telegram] >> text=Finance team').isVisible())

console.log(errs.length ? '\nERRORS:\n' + errs.join('\n') : '\nno console errors')
console.log(f ? `${f} failed` : 'all passed')
await b.close()
process.exit(f ? 1 : 0)
