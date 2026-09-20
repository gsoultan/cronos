/*
 * Assigning a person to a report, through the browser.
 *
 * The server side of this is scripts/live-access.sh, which proves a grant
 * holds on every path into a report. What only a browser can show is that an
 * administrator can reach the control at all — it lived inside the share
 * panel, so "who may read this" was behind a button about links, findable by
 * somebody who already knew where it was.
 *
 * Its own suite rather than an addition to live-portal-check.mjs, because that
 * one runs against a token baked into the build: the panel is drawn from the
 * signed-in session's role, and with no sign-in there is nobody to be an
 * administrator.
 */
import { chromium } from 'playwright'

const B = process.env.BASE ?? 'http://localhost:5174'
const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const page = await browser.newPage()
let fails = 0
const ok = (name, cond) => { console.log(`  ${cond ? 'ok  ' : 'FAIL'} ${name}`); if (!cond) fails++ }

const errors = []
page.on('pageerror', (e) => errors.push(String(e)))

/* -- as an administrator --------------------------------------------------- */

await page.goto(B, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=sign-in]').waitFor({ timeout: 20000 })
await page.fill('input[type=email]', 'admin@acme.example')
await page.fill('input[type=password]', 'correct horse battery staple')
await page.click('[data-testid=sign-in] button[type=submit]')
await page.locator('[data-testid=sidebar], [data-testid=nav-rail], nav').first()
  .waitFor({ timeout: 20000 })

await page.goto(`${B}/reports/billing-summary`, { waitUntil: 'domcontentloaded' })
await page.locator('[data-testid=live-report]').waitFor({ timeout: 20000 })

ok('an administrator is offered the access control on a report',
  await page.locator('[data-testid=access-button]').count() === 1)

await page.click('[data-testid=access-button]')
const panel = page.locator('[data-testid=access-panel]')
await panel.waitFor({ timeout: 15000 })

/* Open and granted-to-nobody are both an empty list and opposite states. The
   server sends `restricted` so the difference is read rather than guessed. */
ok('and a report nobody restricted says it is open to the project',
  (await panel.innerText()).includes('Everybody in this project'))

/* -- assigning somebody ---------------------------------------------------- */

/* By picking them, not by typing an account id. A grant to an id nobody holds
   matches nobody and still restricts the report — so the server refuses one,
   and this control is a list of the people who are actually here. */
await panel.getByLabel('Grant to').click()
await page.getByRole('option', { name: 'Person' }).click()
await panel.getByLabel('Person').click()

/* Whoever the roster offers, rather than a name written here: the list
   excludes people who already hold a grant and anybody disabled, so naming
   one would make this suite depend on what the suites around it did. */
const offered = page.getByRole('option')
await offered.first().waitFor({ timeout: 15000 })
const somebody = (await offered.first().innerText()).trim()
ok('the people it offers are the roster, by name and address',
  somebody.includes('@'))
await offered.first().click()
await panel.getByRole('button', { name: 'Grant' }).click()

await panel.locator('text=Only the people and groups below').waitFor({ timeout: 15000 })
ok('assigning somebody restricts the report to them',
  (await panel.innerText()).includes(somebody.replace(/^.*\(|\)$/g, '')))

/* The grant reads as a permission given to a person, so it names one. The
   stored subject is an account id, which cannot be checked against the person
   who was meant. */
ok('and the grant names the person rather than their account id',
  !(await panel.innerText()).includes('usr_'))

/* -- and taking it back ---------------------------------------------------- */

/* Which also leaves the demo as it was found: a report left restricted here
   would look like a broken catalogue to whatever runs next. */
await panel.locator('button[aria-label^="Remove"]').first().click()
await panel.locator('text=Everybody in this project').waitFor({ timeout: 15000 })
ok('and revoking opens it to the project again', true)

ok(`nothing was thrown (${errors.length})`, errors.length === 0)
if (errors.length) console.log(errors.slice(0, 3).map((e) => `       ${e}`).join('\n'))

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
process.exit(fails ? 1 : 0)
