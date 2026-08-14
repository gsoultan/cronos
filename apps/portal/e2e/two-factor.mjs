/*
 * Drives two-factor enrolment through a real browser.
 *
 * The other checks in this directory drive the API. This one drives the thing a
 * person touches, and it earned its place the first time it ran: it found that
 * the enrolment wizard rendered *inside* the account page, between the password
 * and the sessions, with two sets of Back/Continue controls on screen — and a
 * React key collision from two sibling panels keyed by the same counter. Neither
 * is visible to a type checker, a linter or an API test.
 *
 * The code is computed here, in Node, rather than asked of cronos. A browser
 * check that asks the server what code to type proves the two halves agree with
 * each other, which is exactly what the wizard this replaces did — it accepted
 * any six digits, so it agreed with everything.
 *
 *   bun run --cwd apps/portal e2e
 *
 * Wants a cronos on :8798 with an account, and the portal on :5273 pointed at
 * it. scripts/live-portal-2fa.sh starts both.
 */
import { chromium } from 'playwright'
import { createHmac } from 'node:crypto'

// TOTP computed here, not by cronos: the browser has to agree with something
// that is not the thing under test.
function totp(secret, skew = 0) {
  const b32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const c of secret.replace(/[\s=]/g, '').toUpperCase()) {
    bits += b32.indexOf(c).toString(2).padStart(5, '0')
  }
  const key = Buffer.from((bits.match(/.{8}/g) ?? []).map((b) => parseInt(b, 2)))
  const counter = Math.floor(Date.now() / 30000) + skew
  const msg = Buffer.alloc(8)
  msg.writeBigUInt64BE(BigInt(counter))
  const d = createHmac('sha1', key).update(msg).digest()
  const off = d[d.length - 1] & 0x0f
  const code = ((d.readUInt32BE(off) & 0x7fffffff) % 1e6).toString().padStart(6, '0')
  return code
}

const PORTAL = process.env.PORTAL ?? 'http://localhost:5273'

// Anything that fails here should fail the script, not print and carry on.
function must(ok, what) {
  if (!ok) {
    console.error(`  FAILED ${what}`)
    process.exitCode = 1
    throw new Error(what)
  }
  console.log(`  ok ${what}`)
}

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 1100 } })
const problems = []
page.on('pageerror', (e) => problems.push('pageerror: ' + e.message))
page.on('console', (m) => { if (m.type() === 'error') problems.push('console: ' + m.text().slice(0, 120)) })

await page.goto(PORTAL + '/', { waitUntil: 'networkidle' })
await page.getByTestId('email').fill('ada@acme.example')
await page.getByTestId('password').fill('a-password-they-chose')
await page.getByTestId('submit').click()
await page.waitForTimeout(1200)

await page.goto(PORTAL + '/account', { waitUntil: 'networkidle' })
await page.waitForTimeout(800)

/*
 * What the page offers before anything is set up.
 *
 * These assertions used to live in scripts/security-check.mjs, which ran
 * against sample data and typed "123456" at the verification step — possible
 * only because nothing checked it. When the account page learned to say "this
 * is the sample portal" rather than render four panels about an account that
 * does not exist, that suite stopped being able to see the card at all, and
 * had been failing in CI ever since. Here they run against a real server, in
 * front of the enrolment below that computes a genuine code.
 */
must((await page.getByTestId('two-factor-state').innerText()).trim() === 'Off',
  'two-factor starts off')
must(await page.locator('button:has-text("SMS")').count() === 0,
  'SMS is not offered — a code by text is a code an operator can be talked into')

await page.getByTestId('turn-on-2fa').click()
await page.locator('svg[role="img"] path').first().waitFor({ timeout: 15_000 })

const shown = (await page.locator('code').first().innerText()).replace(/\s/g, '')
must(/^[A-Z2-7]{32}$/.test(shown), 'the key shown is a base32 secret')
must(await page.locator('svg[role="img"] path').count() > 0, 'a QR code was drawn')

await page.getByRole('button', { name: 'Continue' }).click()
await page.waitForTimeout(500)
await page.getByTestId('factor-verify').fill(totp(shown))
await page.getByRole('button', { name: 'Verify' }).click()

// Waited for rather than slept past. A fixed pause is a race that passes on a
// warm dev server and fails on a cold one, and the failure it produces —
// "no recovery codes" — points at the wrong thing entirely.
await page.getByTestId('recovery-codes').waitFor({ timeout: 15_000 }).catch(async () => {
  const shown = await page.locator('[role="alert"], .text-danger').allInnerTexts()
  throw new Error(`verification did not produce recovery codes: ${shown.join(' | ') || '(no message on the page)'}`)
})

const codes = await page.getByTestId('recovery-codes').locator('li').allInnerTexts()
must(codes.length === 10, 'ten recovery codes came back')
must(codes.every((c) => /^[0-9A-HJKMNP-TV-Z]{5}-[0-9A-HJKMNP-TV-Z]{5}$/.test(c)),
  'each is five-hyphen-five from an unambiguous alphabet')

await page.getByTestId('codes-saved').check()
await page.getByRole('button', { name: 'Finish' }).click()
await page.getByTestId('two-factor-state').waitFor({ timeout: 15_000 })
must((await page.getByTestId('two-factor-state').innerText()).trim() === 'On',
  'the account page says two-factor is on')

// And a fresh sign-in now needs the code.
await page.evaluate(() => localStorage.clear())
await page.goto(PORTAL + '/', { waitUntil: 'networkidle' })
await page.getByTestId('email').fill('ada@acme.example')
await page.getByTestId('password').fill('a-password-they-chose')
await page.getByTestId('submit').click()
await page.waitForTimeout(1000)
must(await page.getByTestId('factor-code').count() === 1,
  'a fresh sign-in asks for a code')

await page.getByTestId('factor-code').fill(totp(shown, 1))
await page.getByTestId('submit').click()
await page.waitForTimeout(1500)
must(await page.getByTestId('account-link').count() === 1, 'and the code signs in')

must(problems.length === 0, `no console errors${problems.length ? ':\n    ' + problems.join('\n    ') : ''}`)
console.log('\nAll of it worked.')
await browser.close()
