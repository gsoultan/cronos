/*
 * Signing in, against a real cronosd with a real user in it.
 *
 * The portal is served with no token baked in, so it has to do what a person
 * would: show a sign-in page, take an email and a password, and come back with
 * a session that works.
 */
import { chromium } from 'playwright'

const B = process.env.BASE ?? 'http://localhost:5174'
const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const page = await browser.newPage()
let fails = 0
const ok = (name, cond) => { console.log(`  ${cond ? 'ok  ' : 'FAIL'} ${name}`); if (!cond) fails++ }

await page.goto(B, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=sign-in]').waitFor({ timeout: 20000 })

/* A server is configured and nobody is signed in: the sign-in page and nothing
   else. No shell, no navigation to pages that would only 401. */
ok('a configured portal with no session shows sign-in',
  await page.locator('[data-testid=sign-in]').isVisible())
ok('and nothing else', await page.locator('[data-testid=sidebar]').count() === 0)

/* One message for every failure. Telling "no such account" apart from "wrong
   password" is how somebody learns which addresses are registered. */
await page.fill('[data-testid=email] input, input[type=email]', 'dewi@acme.example')
await page.fill('[data-testid=password] input, input[type=password]', 'not the password')
await page.click('[data-testid=submit]')
await page.locator('[data-testid=sign-in-error]').waitFor({ timeout: 15000 })
const wrongPassword = await page.locator('[data-testid=sign-in-error]').innerText()

await page.fill('[data-testid=email] input, input[type=email]', 'nobody@acme.example')
await page.fill('[data-testid=password] input, input[type=password]', 'correct horse battery staple')
await page.click('[data-testid=submit]')
await page.waitForTimeout(1500)
const unknownEmail = await page.locator('[data-testid=sign-in-error]').innerText()

ok('a wrong password and an unknown email read identically',
  wrongPassword === unknownEmail && wrongPassword.length > 0)

/* -- The real thing -------------------------------------------------------- */
await page.fill('[data-testid=email] input, input[type=email]', 'dewi@acme.example')
await page.fill('[data-testid=password] input, input[type=password]', 'correct horse battery staple')
await page.click('[data-testid=submit]')

await page.locator('[data-testid=sidebar]').waitFor({ timeout: 20000 })
ok('the right password reaches the portal', true)

await page.goto(`${B}/reports/billing-summary`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=live-report]').waitFor({ timeout: 20000 })
ok('the session reads real reports',
  (await page.locator('[data-testid=live-report]').innerText()).includes('154,651.50'))

/* A session survives a reload, or somebody signs in on every navigation. */
await page.reload({ waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=live-report]').waitFor({ timeout: 20000 })
ok('the session survives a reload', true)

/* And an expired one returns to sign-in rather than to an error nobody can
   act on. A portal that says "unauthorised" and offers no way out is one
   somebody reloads until it works. */
await page.evaluate(() => localStorage.setItem('cronos.token', 'v1.bm9wZQ.bm9wZQ'))
await page.goto(`${B}/reports/billing-summary`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=sign-in]').waitFor({ timeout: 20000 })
ok('an expired session returns to sign-in', true)

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
process.exit(fails ? 1 : 0)
