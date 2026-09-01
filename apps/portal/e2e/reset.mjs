/*
 * Getting back in after forgetting a password, as a person does it.
 *
 * Driven by scripts/live-reset.sh, which stands up a mail server, reads the
 * link out of the inbox and passes it in. Everything worth checking here is
 * what happens *around* the new password: a session somebody else is holding,
 * a link pressed twice, and a second factor that a mailbox must not stand in
 * for.
 */
import { chromium } from 'playwright'
import { createHmac } from 'node:crypto'

// Computed here, not asked of cronos: a check that asks the server what code to
// type proves the two halves agree with each other and nothing else.
function totp(secret) {
  const b32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const c of secret.replace(/[\s=]/g, '').toUpperCase()) {
    bits += b32.indexOf(c).toString(2).padStart(5, '0')
  }
  const key = Buffer.from((bits.match(/.{8}/g) ?? []).map((b) => parseInt(b, 2)))
  const msg = Buffer.alloc(8)
  msg.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30000)))
  const d = createHmac('sha1', key).update(msg).digest()
  const off = d[d.length - 1] & 0x0f
  return ((d.readUInt32BE(off) & 0x7fffffff) % 1e6).toString().padStart(6, '0')
}

const PORTAL = process.env.PORTAL ?? 'http://localhost:5275'
const API = process.env.API ?? 'http://localhost:8811'
const LINK = process.env.LINK ?? ''

const ok = (m) => console.log(`  \x1b[32mok\x1b[0m ${m}`)
const die = (m) => { console.error(`  \x1b[31mFAILED\x1b[0m ${m}`); process.exit(1) }

const login = (password) =>
  fetch(`${API}/v1/auth/login`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ email: 'ada@acme.example', password }),
  })

/* A session held before the reset — the one belonging to whoever is already in
   the account, which is half of why somebody asks for a reset at all. */
const before = await login('the-password-she-forgot')
if (!before.ok) die('the account cannot sign in with the password it started with')
const held = (await before.json()).token

/*
 * A second factor, enrolled before any of this.
 *
 * The property it buys is the one a reset most easily destroys: a reset proves
 * control of a mailbox, and a second factor exists precisely because control of
 * a mailbox is not enough. If setting a new password let somebody past it, the
 * reset would be the way around the strongest control this product has.
 */
const authed = { authorization: `Bearer ${held}`, 'content-type': 'application/json' }
const started = await fetch(`${API}/v1/auth/factor/start`, { method: 'POST', headers: authed })
if (!started.ok) die(`could not begin enrolling a second factor: ${started.status}`)
const { secret: factor } = await started.json()
const confirmed = await fetch(`${API}/v1/auth/factor/confirm`, {
  method: 'POST', headers: authed, body: JSON.stringify({ code: totp(factor) }),
})
if (!confirmed.ok) die(`could not enrol a second factor: ${await confirmed.text()}`)

const withFactor = await (await login('the-password-she-forgot')).json()
if (withFactor.factorRequired !== true) {
  die('the account has a second factor and a password alone still signed in')
}
ok('the account has a second factor, and a password alone does not get in')

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
const problems = []
page.on('pageerror', (e) => problems.push('pageerror: ' + e.message))
page.on('console', (m) => { if (m.type() === 'error') problems.push('console: ' + m.text().slice(0, 120)) })

/* The link as it arrives, pointed at this run's portal. The fragment is the
   part that matters and is preserved exactly. */
const target = LINK.replace(/^https?:\/\/[^/]+/, PORTAL)
await page.goto(target, { waitUntil: 'networkidle' })
await page.waitForTimeout(800)

if (await page.getByTestId('reset-password').count() !== 1) {
  die(`the link did not open a page that asks for a password: ${(await page.locator('body').innerText()).slice(0, 200)}`)
}
ok('the link opens a page asking for a new password')

// And the secret is out of the address bar, so it is not in history or in a
// screenshot of somebody's browser.
if (page.url().includes('secret=')) die(`the secret is still in the address bar: ${page.url()}`)
ok('and the secret is out of the address bar')

await page.getByTestId('reset-password').fill('short')
await page.getByTestId('reset-again').fill('short')
if (!(await page.getByTestId('reset-submit').isDisabled())) {
  die('a five-character password could be submitted')
}
ok('a password that is too short cannot be submitted')

await page.getByTestId('reset-password').fill('the-password-she-chose')
await page.getByTestId('reset-again').fill('the-password-she-typed-wrong')
if (!(await page.getByTestId('reset-submit').isDisabled())) {
  die('two different passwords could be submitted')
}
await page.getByTestId('reset-again').fill('the-password-she-chose')
await page.getByTestId('reset-submit').click()
await page.getByTestId('reset-done').waitFor({ timeout: 15_000 }).catch(async () => {
  die(`setting the password did not work: ${(await page.locator('body').innerText()).slice(0, 240)}`)
})
ok('and one she typed twice is accepted')

// -- What the reset did --------------------------------------------------------

if ((await login('the-password-she-forgot')).status !== 401) {
  die('the password she forgot still works')
}
if (!(await login('the-password-she-chose')).ok) {
  die('the password she just chose does not work')
}
ok('the old password has stopped working and the new one has started')

/*
 * The session somebody else was holding.
 *
 * "I cannot get in" and "somebody else is in" are the same sentence from
 * outside, so a reset that leaves the intruder signed in has recovered
 * nothing. The check is bounded by the standing cache, which is five seconds
 * by design — a database round trip per request to ask whether a session is
 * still a session is a cost everybody pays so a rare event is instant.
 */
let cut = false
for (let i = 0; i < 12; i++) {
  const r = await fetch(`${API}/v1/people`, { headers: { authorization: `Bearer ${held}` } })
  if (r.status === 401) { cut = true; break }
  await new Promise((r) => setTimeout(r, 1000))
}
if (!cut) die('a session from before the reset still works — whoever was in is still in')
ok('and the session from before it has ended')

/*
 * Pressed twice: a forwarded mail, a back button, a prefetch.
 *
 * Away and back, rather than straight to the link. The page took the secret
 * out of the address bar when it read it, so the browser is sitting on
 * `/reset` — and going from there to `/reset#secret=…` changes only the
 * fragment, which is a same-document navigation that React never sees. The
 * test would then be looking at the success panel it left behind and calling
 * it a missing field.
 */
await page.goto(PORTAL + '/', { waitUntil: 'networkidle' })
await page.goto(target, { waitUntil: 'networkidle' })
await page.waitForTimeout(800)
await page.getByTestId('reset-password').fill('a-password-somebody-else-picked')
await page.getByTestId('reset-again').fill('a-password-somebody-else-picked')
await page.getByTestId('reset-submit').click()
await page.waitForTimeout(1500)

if (await page.getByTestId('reset-done').count() !== 0) {
  die('the link worked a second time')
}
const refusal = (await page.locator('[role="alert"]').allInnerTexts()).join(' ')
if (!/expired|already been used/.test(refusal)) {
  die(`the second attempt failed without saying why: ${refusal || '(nothing on the page)'}`)
}
if ((await login('a-password-somebody-else-picked')).ok) {
  die('the second attempt changed the password anyway')
}
ok('a second press is refused, and says the link is spent')

/*
 * And a mailbox is still not a second factor.
 *
 * The whole reason a reset must not hand back a session: somebody who reads the
 * email would otherwise be inside the account, and the code on the phone would
 * never have been asked for.
 */
const after = await (await login('the-password-she-chose')).json()
if (after.factorRequired !== true) {
  die('signing in after a reset did not ask for the second factor — a reset is a way around it')
}
if (after.token) die('a reset handed back a session; proving control of a mailbox is not signing in')
ok('and signing in with the new password still asks for the code')

// The 410 from the second press above is deliberate and a browser logs it as a
// console error. Named exactly, so a 410 from anywhere else still counts.
const unexpected = problems.filter((p) => !/410/.test(p))
if (unexpected.length > 0) die(`console errors:\n    ${unexpected.join('\n    ')}`)

console.log('\n  \x1b[32ma forgotten password has a way back\x1b[0m\n')
await browser.close()
