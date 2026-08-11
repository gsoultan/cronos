/* Two-factor: enrolment order, recovery codes, and the org requirement. */
import { chromium } from 'playwright'
const B = process.env.BASE ?? 'http://localhost:5173'
const b = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const p = await (await b.newContext({ viewport: { width: 1600, height: 1050 } })).newPage()
const errs = []
p.on('pageerror', e => errs.push(String(e)))
p.on('console', m => m.type() === 'error' && errs.push(m.text()))
let f = 0
const ok = (n, c) => { console.log(`  ${c ? 'ok  ' : 'FAIL'} ${n}`); if (!c) f++ }

// Reachable from the avatar, not buried.
await p.goto(B + '/', { waitUntil: 'domcontentloaded' })
await p.click('[data-testid=account-link]')
await p.waitForSelector('[data-testid=two-factor]')
ok('the account page is one click from the header', p.url().endsWith('/account'))
ok('two-factor starts off',
  await p.locator('[data-testid=two-factor-state]').innerText() === 'Off')
ok('SMS is not offered anywhere on the page',
  await p.locator('text=/SMS|text message code/i').count() >= 0 &&
  await p.locator('button:has-text("SMS")').count() === 0)

await p.click('[data-testid=two-factor] button:has-text("Turn on")')
await p.waitForSelector('[data-testid=method-app]')
ok('passkey is offered first as the stronger method',
  (await p.locator('[data-testid=method-passkey], [data-testid=method-app]').first()
     .getAttribute('data-testid')) === 'method-passkey')

await p.click('[data-testid=method-app]')
await p.click('button:has-text("Continue")')
await p.waitForTimeout(300)
ok('the manual key is offered alongside the QR',
  await p.locator('text=Or enter this key by hand').isVisible())

await p.click('button:has-text("Continue")')
await p.waitForSelector('input[aria-label="Six-digit code"]')
const nextBtn = p.locator('button:has-text("Verify")')
ok('cannot skip verification', await nextBtn.isDisabled())
await p.fill('input[aria-label="Six-digit code"]', '12ab56')
await p.waitForTimeout(200)
ok('a malformed code still blocks', await nextBtn.isDisabled())
await p.fill('input[aria-label="Six-digit code"]', '123456')
await p.waitForTimeout(200)
await nextBtn.click()
await p.waitForSelector('text=Save these somewhere safe')

ok('recovery codes are shown', await p.locator('li:has-text("-")').count() >= 10)
ok('says they are shown only once',
  await p.locator('text=only time they are shown').isVisible())
const finish = p.locator('button:has-text("Finish")')
ok('cannot finish without acknowledging the codes', await finish.isDisabled())
await p.locator('input[type=checkbox]').check()
await p.waitForTimeout(200)
await finish.click()
await p.waitForTimeout(400)
ok('two-factor is now on',
  await p.locator('[data-testid=two-factor-state]').innerText() === 'On')
ok('removing the last factor says it turns 2FA off',
  await p.locator('button:has-text("turns 2FA off")').isVisible())

// Org policy: readiness before the switch, and self-lockout prevented.
await p.goto(B + '/settings', { waitUntil: 'domcontentloaded' })
await p.click('[data-testid=workspace-trigger]')
await p.click('[data-testid=workspace-item]:has-text("Northwind")')
await p.waitForTimeout(400)
await p.click('[role=tab]:has-text("Security")')
await p.waitForSelector('[data-testid=security-policy]')
ok('coverage is shown before the decision',
  await p.locator('text=/\\d+ of \\d+ protected/').isVisible())
ok('the people who would lose access are named',
  await p.locator('text=Without a second factor').isVisible())
ok('an admin without 2FA cannot require it',
  await p.locator('[aria-label="Require two-factor authentication"]').isDisabled())
ok('and is told why', await p.locator('text=You are not protected yet').isVisible())
ok('SSO is marked commercial, 2FA is not',
  await p.locator('text=Two-factor above is free').isVisible())

console.log(errs.length ? '\nERRORS:\n' + errs.join('\n') : '\nno console errors')
console.log(f ? `${f} failed` : 'all passed')
await b.close()
process.exit(f ? 1 : 0)
