/*
 * Drives <cronos-report> in a real browser against a stub API.
 *
 * A host page is the thing being tested, not the component in isolation: the
 * claims worth checking are about what survives contact with someone else's
 * CSS, someone else's data, and someone else's expired token.
 */
import { createServer } from 'node:http'
import { readFileSync } from 'node:fs'
import { chromium } from 'playwright'

/* A customer name straight from our customer's database — which is to say,
   from whoever signed up. This is the string that decides whether the package
   needed an escaping rule or a design that has no escaping. */
const HOSTILE = '<img src=x onerror="window.__pwned=1">Aurora'

const payload = (filtered) => ({
  title: 'Monthly invoice statement',
  filters: [
    { name: 'period', label: 'Period', type: 'date' },
    { name: 'status', label: 'Status', type: 'enum' },
  ],
  blocks: [
    {
      kind: 'stat', title: 'Total billed', value: filtered ? '€12.1M' : '€49.9M',
      delta: { value: '6.4%', dir: 'up', good: true, label: 'vs last month' },
      coverage: { applied: ['period', 'status'] },
    },
    {
      kind: 'stat', title: 'Open shipments', value: '318',
      // Bound to no field in this block's dataset — the case the report format
      // says the interface must announce.
      coverage: { applied: ['period'], ignored: ['status'] },
    },
    {
      kind: 'chart', chart: 'bar', title: 'Billed by month',
      series: [
        { label: 'May', value: 40, formatted: '€4.0M' },
        { label: 'Jun', value: 80, formatted: '€8.0M' },
      ],
      coverage: { applied: ['period'], ignored: ['status'] },
    },
    {
      kind: 'table', title: 'Invoices', total: 1284,
      columns: [{ label: 'Customer' }, { label: 'Amount', align: 'right' }],
      rows: [[HOSTILE, '€1,234.56']],
      coverage: { applied: ['period', 'status'] },
    },
  ],
})

const HOST = `<!doctype html><meta charset="utf-8">
<style>
  /* A host page with opinions, which is every host page. */
  .panel { display: none !important }
  table { border-collapse: separate; border: 8px solid lime }
  p { font-size: 40px }
</style>
<div id="stage"><cronos-report id="r" endpoint="" token="tok" report="monthly"></cronos-report></div>
<script type="module" src="/cronos-embed.js"></script>`

let requests = 0
let lastBody = null
let unauthorized = false
let future = false

const server = createServer((req, res) => {
  if (req.url === '/' || req.url === '') {
    res.writeHead(200, { 'content-type': 'text/html' })
    return res.end(HOST)
  }
  if (req.url === '/cronos-embed.js') {
    res.writeHead(200, { 'content-type': 'text/javascript' })
    return res.end(readFileSync('dist/cronos-embed.js'))
  }
  if (req.url.startsWith('/v1/embed/reports/')) {
    requests++
    let body = ''
    req.on('data', (c) => (body += c))
    return req.on('end', () => {
      lastBody = body
      if (unauthorized) {
        res.writeHead(401, { 'content-type': 'application/json' })
        return res.end(JSON.stringify({ error: 'This report link has expired.' }))
      }
      res.writeHead(200, { 'content-type': 'application/json' })
      if (future) {
        // A shape only a later cronos knows how to draw.
        return res.end(JSON.stringify({
          title: 'From the future',
          blocks: [{ kind: 'sankey', title: 'Flows', nodes: [], links: [] }],
        }))
      }
      res.end(JSON.stringify(payload(body.includes('overdue'))))
    })
  }
  res.writeHead(404).end()
})

await new Promise((r) => server.listen(0, r))
const base = `http://localhost:${server.address().port}`

const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const page = await browser.newPage()
let fails = 0
const ok = (name, cond) => { console.log(`  ${cond ? 'ok  ' : 'FAIL'} ${name}`); if (!cond) fails++ }

await page.goto(base, { waitUntil: 'domcontentloaded' })
await page.evaluate((b) => document.querySelector('#r').setAttribute('endpoint', b), base)
const report = page.locator('#r')
await report.locator('.panel').first().waitFor()

/* -- It renders ----------------------------------------------------------- */
ok('renders every block', await report.locator('.panel').count() === 4)
ok('the headline value is shown', (await report.locator('.stat').first().innerText()) === '€49.9M')
ok('a delta is coloured by meaning, not direction',
  await report.locator('.delta b.up').first().isVisible())
ok('the table says what it is showing',
  (await report.locator('.panel', { hasText: 'Invoices' }).innerText()).includes('1 of 1284'))

/* -- The report format's promise ------------------------------------------ */
const shipments = report.locator('.panel', { hasText: 'Open shipments' })
ok('a block says which filter does not reach it',
  (await shipments.innerText()).includes('Not affected by Status'))
ok('a fully covered block says nothing',
  !(await report.locator('.panel', { hasText: 'Total billed' }).innerText()).includes('Not affected'))

/* -- Hostile data --------------------------------------------------------- */
ok('a name from a customer database is text, not markup',
  await page.evaluate(() => window.__pwned === undefined))
ok('and it is still displayed in full',
  (await report.locator('td').first().innerText()).includes('onerror'))

/* -- The host page cannot break it ---------------------------------------- */
ok('the host page cannot hide our panels',
  await report.locator('.panel').first().isVisible())
ok("the host page's table styling does not reach in",
  await report.locator('table').evaluate((t) => getComputedStyle(t).borderTopWidth) === '0px')

/* -- Theming crosses the boundary on purpose ------------------------------ */
await page.evaluate(() => document.querySelector('#r').style.setProperty('--cr-accent', 'rgb(255, 0, 0)'))
ok('custom properties theme it',
  await report.locator('.fill').first().evaluate((f) => getComputedStyle(f).backgroundColor) === 'rgb(255, 0, 0)')

/* -- Filters -------------------------------------------------------------- */
const before = requests
await page.evaluate(() => {
  document.querySelector('#r').filters = { status: { op: 'in', values: ['overdue'] } }
})
await page.waitForFunction(() => document.querySelector('#r').shadowRoot
  .querySelector('.stat')?.textContent === '€12.1M')
ok('setting filters refetches', requests > before)
ok('filters are sent as a request, not applied locally',
  lastBody.includes('"op":"in"') && lastBody.includes('overdue'))

/* -- Failure -------------------------------------------------------------- */
unauthorized = true
await page.evaluate(() => { document.querySelector('#r').filters = {} })
await report.locator('.msg.err').waitFor()
ok("an expired link says so in the server's words",
  (await report.locator('.msg.err').innerText()).includes('expired'))

/* -- Removal -------------------------------------------------------------- */
await page.evaluate(() => document.querySelector('#r').remove())
ok('removing it does not throw', await page.evaluate(() => true))

/* A host page pins a bundle version and cronos ships features afterwards, so a
   block kind this build has never heard of is a normal condition rather than a
   bug. It used to fall through to the table renderer and throw on columns.map,
   which took the whole host page down with it. */
unauthorized = false // the 401 case above is finished with
future = true
const fresh = await browser.newPage()
const thrown = []
fresh.on('pageerror', (e) => thrown.push(String(e)))
await fresh.goto(base, { waitUntil: 'domcontentloaded' })
await fresh.evaluate((b) => document.querySelector('#r').setAttribute('endpoint', b), base)
await fresh.locator('#r').locator('.panel').first().waitFor()

ok('a block kind from a newer server renders a message, not an exception',
  (await fresh.locator('#r').locator('.grid').innerText()).includes('newer viewer'))
ok('and nothing was thrown into the host page', thrown.length === 0)
await fresh.close()

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
server.close()
process.exit(fails ? 1 : 0)
