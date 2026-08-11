/*
 * The component against the real server.
 *
 * embed-check.mjs proves the component renders whatever it is handed;
 * service_test.go proves the server computes the right numbers. Neither proves
 * they agree about the shape in between, and that seam — a Go field name, a
 * JSON tag, a TypeScript interface — is where an integration actually breaks.
 *
 * Started by scripts/live-embed.sh, which builds cronosd, seeds it and mints
 * the tokens.
 */
import { createServer } from 'node:http'
import { readFileSync } from 'node:fs'
import { chromium } from 'playwright'

const API = process.env.CRONOS_BASE
const TOKEN = process.env.CRONOS_TOKEN
const TOKEN_C2 = process.env.CRONOS_TOKEN_C2
const PORT = Number(process.env.LIVE_PORT ?? 5199)

if (!API || !TOKEN) {
  console.error('run this through scripts/live-embed.sh')
  process.exit(2)
}

/* The host page. Served from its own origin so the request is genuinely
   cross-origin — which is what an ISV's page is, and the only way the CORS
   allow-list is under test rather than bypassed. */
const HOST = `<!doctype html><meta charset="utf-8">
<div id="stage"></div>
<script type="module">
  import '/cronos-embed.js'
  const el = document.createElement('cronos-report')
  el.setAttribute('endpoint', ${JSON.stringify(API)})
  el.setAttribute('token', ${JSON.stringify(TOKEN)})
  el.setAttribute('report', 'billing-summary')
  el.filters = { period: { op: 'between', values: ['2026-01-01', '2026-12-31'] } }
  document.getElementById('stage').append(el)
  window.swap = (t) => el.setAttribute('token', t)
</script>`

const page = createServer((req, res) => {
  if (req.url === '/cronos-embed.js') {
    res.writeHead(200, { 'content-type': 'text/javascript' })
    return res.end(readFileSync('packages/embed/dist/cronos-embed.js'))
  }
  res.writeHead(200, { 'content-type': 'text/html' }).end(HOST)
})

/* A fixed port, because cronosd's CORS allow-list has to name this origin
   before either process starts. That makes "something is already listening"
   a real condition, and listen's callback simply never fires on EADDRINUSE —
   so the run hangs instead of saying why. */
await new Promise((resolve, reject) => {
  page.once('error', (err) => reject(err.code === 'EADDRINUSE'
    ? new Error(`port ${PORT} is in use — a previous run left its page server behind`)
    : err))
  page.listen(PORT, resolve)
})

const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
/* Closed whatever happens below. A thrown assertion used to leave this server
   listening, and the next run met a port that answered with a stale page. */
process.on('exit', () => { page.close(); browser.close() })

const tab = await browser.newPage()
let fails = 0
const ok = (name, cond) => { console.log(`  ${cond ? 'ok  ' : 'FAIL'} ${name}`); if (!cond) fails++ }

const report = tab.locator('cronos-report')
const text = () =>
  tab.evaluate(() => document.querySelector('cronos-report').shadowRoot.textContent)

await tab.goto(`http://localhost:${PORT}/`, { waitUntil: 'domcontentloaded' })
await report.locator('.panel').first().waitFor({ timeout: 15000 })

/* -- It agrees with the server -------------------------------------------- */
const body = await text()
ok('renders cronos-computed blocks', (await report.locator('.panel').count()) === 4)
ok('the stat carries the total the engine computed (60,171.25)',
  body.includes('60,171.25'))
ok('a block filter narrows one tile only (4,120.75 outstanding)',
  body.includes('4,120.75'))
ok('the chart labels months rather than truncated dates',
  body.includes('Jul 2026') && !body.includes('2026-07-01'))
ok('table headings come from the dataset labels',
  body.includes('Issued') && body.includes('Amount'))

/* -- The report format's promise, computed by the server ------------------ */
ok('a block says the filter that misses it does not apply',
  body.includes('Not affected by Region'))

/* -- Cross-origin, through the allow-list --------------------------------- */
ok('the request was genuinely cross-origin',
  new URL(API).port !== String(PORT))

/* -- Row scope, end to end ------------------------------------------------ */
await tab.evaluate((t) => window.swap(t), TOKEN_C2)
await tab.waitForFunction(() =>
  document.querySelector('cronos-report').shadowRoot.textContent.includes('78,950.25'),
  null, { timeout: 15000 })
const second = await text()
ok('another customer sees their own total (78,950.25)', second.includes('78,950.25'))
ok('and never the first customer\'s', !second.includes('60,171.25'))

/* -- A rejected token reaches the person, in words ------------------------ */
await tab.evaluate(() => window.swap('v1.bm9wZQ.bm9wZQ'))
await report.locator('.msg.err').waitFor({ timeout: 10000 })
ok("a refused token says so in the server's own words",
  (await report.locator('.msg.err').innerText()).includes('no longer valid'))

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
process.exit(fails ? 1 : 0)
