/*
 * Every page of a connected portal, looked at.
 *
 * The suites beside this one drive sample data, which is what makes the
 * interface workable before a server exists and is not what anybody deploys.
 * This signs in against a real cronos and walks the navigation, reporting any
 * console error and any API call that did not answer 2xx.
 *
 * It exists because until scripts/dev.sh started connecting the two halves,
 * nobody had ever seen these pages against a server — and the first look found
 * that the Reports page, the landing page of the whole product, had no live
 * path at all: it read the sample directory, filtered it by the real project,
 * and showed "No reports yet" beside three the server was serving.
 *
 *   ./scripts/dev.sh            # in one terminal
 *   bun run --cwd apps/portal walk
 *
 * Runs against a fresh deployment or a set-up one, so it can be run twice.
 */
import { chromium } from 'playwright'

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1400, height: 1000 } })

const problems = []
page.on('pageerror', (e) => problems.push(`  pageerror  ${e.message}`))
page.on('console', (m) => {
  if (m.type() === 'error') problems.push(`  console    ${m.text().slice(0, 180)}`)
})
// Every API call the page makes, and what came back.
const calls = []
page.on('response', (r) => {
  const u = new URL(r.url())
  if (u.port === '8080') calls.push(`${r.status()} ${u.pathname}`)
})

await page.goto('http://localhost:5173/', { waitUntil: 'networkidle' })
await page.waitForTimeout(1200)

// Set up if it is a fresh deployment, sign in if it is not — so this can be run
// twice without deleting the database in between.
if (await page.getByTestId('sign-in').count() === 1) {
  await page.getByTestId('email').fill('you@example.com')
  await page.getByTestId('password').fill('a-development-password')
  await page.getByTestId('submit').click()
  await page.waitForTimeout(2000)
} else {
  await page.getByTestId('setup-email').fill('you@example.com')
  await page.getByTestId('setup-name').fill('Dev Person')
  await page.getByTestId('setup-org').fill('Acme')
  await page.getByTestId('setup-project').fill('Finance')
  await page.getByTestId('setup-password').fill('a-development-password')
  await page.getByTestId('setup-password-again').fill('a-development-password')
  await page.getByTestId('setup-submit').click()
  await page.waitForTimeout(2500)
}

for (const [name, path] of [
  ['reports', '/'],
  ['data', '/data'],
  ['schedules', '/schedules'],
  ['activity', '/activity'],
  ['settings', '/settings'],
  ['account', '/account'],
]) {
  problems.length = 0
  calls.length = 0
  await page.goto('http://localhost:5173' + path, { waitUntil: 'networkidle' })
  await page.waitForTimeout(1400)
  await page.screenshot({ path: `/tmp/cronos-shots/dev-${name}.png`, fullPage: true })

  const body = (await page.evaluate(() => document.body.innerText))
    .split('\n').map((l) => l.trim()).filter(Boolean)
  // Skip the nav, which is the same on every page.
  const main = body.slice(body.indexOf('Settings') + 1 >= 0 ? 12 : 0, 40)

  const bad = calls.filter((c) => !c.startsWith('2') && !c.startsWith('304'))
  console.log(`\n=== ${name} (${path}) ===`)
  console.log('  ' + main.slice(0, 10).join(' · '))
  if (bad.length) console.log('  failed calls: ' + [...new Set(bad)].join(', '))
  if (problems.length) console.log(problems.join('\n'))
}

await browser.close()
