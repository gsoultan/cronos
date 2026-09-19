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
    { name: 'period', label: 'Period', type: 'date', control: 'range' },
    { name: 'status', label: 'Status', type: 'enum', values: ['sent', 'overdue', 'paid'], control: 'dropdown' },
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
      kind: 'chart', chart: 'bar', title: 'Billed by carrier', series: [],
      stacked: true,
      groups: [
        { label: 'Aurora', slot: 0, bars: [
          { label: 'May', value: 30, formatted: '€3.0M' },
          { label: 'Jun', value: 50, formatted: '€5.0M' },
        ] },
        // The padded zero: Baltic had no May at all, and a stack cannot line
        // its segments up unless the server says so.
        { label: 'Baltic', slot: 1, bars: [
          { label: 'May', value: 0, formatted: '€0' },
          { label: 'Jun', value: 30, formatted: '€3.0M' },
        ] },
      ],
      totals: [
        { label: 'May', value: 30, formatted: '€3.0M' },
        { label: 'Jun', value: 80, formatted: '€8.0M' },
      ],
      yAxis: { min: 0, max: 80, ticks: [
        { at: 0, label: '€0' }, { at: 0.5, label: '€4.0M' }, { at: 1, label: '€8.0M' },
      ] },
    },
    {
      kind: 'chart', chart: 'line', title: 'Trend', series: [
        { label: 'May', value: 40, formatted: '€4.0M' },
        { label: 'Jun', value: 80, formatted: '€8.0M' },
      ],
      yAxis: { min: 0, max: 80, ticks: [{ at: 0, label: '€0' }, { at: 1, label: '€8.0M' }] },
    },
    {
      kind: 'chart', chart: 'donut', title: 'Share', series: [
        { label: HOSTILE, value: 70, formatted: '€7.0M' },
        { label: 'Baltic', value: 30, formatted: '€3.0M' },
      ],
    },
    {
      kind: 'chart', chart: 'bubble', title: 'Paid against billed', series: [],
      points: [
        { label: 'Aurora', x: 10, y: 40, fx: '€1.0M', fy: '€4.0M', weight: 1, size: '40', slot: 0 },
        { label: 'Baltic', x: 20, y: 10, fx: '€2.0M', fy: '€1.0M', weight: 0.2, size: '12', slot: 1 },
      ],
      xAxis: { min: 0, max: 20, ticks: [{ at: 0, label: '€0' }, { at: 1, label: '€2.0M' }] },
      yAxis: { min: 0, max: 40, ticks: [{ at: 0, label: '€0' }, { at: 1, label: '€4.0M' }] },
    },
    {
      kind: 'chart', chart: 'map', title: 'Depots', series: [],
      map: {
        bounds: { minX: 0.49, minY: 0.32, maxX: 0.5, maxY: 0.34 },
        layers: ['polygon', 'heat', 'bubble', 'flow'],
        shapes: [
          { label: 'England', path: 'M0.49 0.32L0.5 0.32L0.5 0.34L0.49 0.34Z',
            value: 1500, formatted: '1,500', step: 5 },
        ],
        markers: [
          { label: 'London', x: 0.4996, y: 0.3325, value: 1200, formatted: '1,200', weight: 1 },
          { label: 'Bristol', x: 0.4928, y: 0.3329, value: 300, formatted: '300', weight: 0.25 },
        ],
        arcs: [
          { label: 'London → Bristol', x1: 0.4996, y1: 0.3325, x2: 0.4928, y2: 0.3329,
            value: 300, formatted: '300', weight: 0.25 },
        ],
        legend: [{ step: 5, from: '700', to: '1,500' }],
        // A tile template the stub server answers, so no request leaves the
        // machine and the attribution rule is still exercised.
        tiles: { url: `${'{z}'}/${'{x}'}/${'{y}'}.png`, attribution: '© OpenStreetMap contributors', maxZoom: 19 },
      },
    },
    {
      kind: 'chart', chart: 'combo', title: 'Billed and margin', series: [],
      tracks: [
        { label: 'Billed', slot: 0, draw: 'bar', bars: [
          { label: 'May', value: 40, formatted: '€4.0M' },
          { label: 'Jun', value: 80, formatted: '€8.0M' },
        ] },
        // On its own scale, which a measure opts into per measure.
        { label: 'Margin', slot: 1, draw: 'line', secondary: true, bars: [
          { label: 'May', value: 12, formatted: '12%' },
          { label: 'Jun', value: 18, formatted: '18%' },
        ] },
      ],
      yAxis: { min: 0, max: 80, ticks: [{ at: 0, label: '€0' }, { at: 1, label: '€8.0M' }] },
      axis2: { min: 0, max: 20, ticks: [{ at: 0, label: '0%' }, { at: 1, label: '20%' }] },
    },
    {
      kind: 'chart', chart: 'funnel', title: 'Conversion', series: [],
      stages: [
        { label: 'Quoted', value: 1000, formatted: '1,000', share: 1 },
        { label: 'Ordered', value: 400, formatted: '400', share: 0.4, drop: '-60.0%' },
        { label: 'Paid', value: 90, formatted: '90', share: 0.09, drop: '-77.5%' },
      ],
    },
    {
      kind: 'chart', chart: 'waterfall', title: 'Movement', series: [],
      steps: [
        { label: 'Opening', value: 100, formatted: '€100k', start: 0, end: 100, sign: 1 },
        { label: 'Billed', value: 60, formatted: '€60k', start: 100, end: 160, sign: 1 },
        { label: 'Paid', value: -40, formatted: '-€40k', start: 160, end: 120, sign: -1 },
        { label: 'Total', value: 120, formatted: '€120k', start: 0, end: 120, sign: 0, total: true },
      ],
      yAxis: { min: 0, max: 160, ticks: [{ at: 0, label: '€0' }, { at: 1, label: '€160k' }] },
    },
    {
      kind: 'chart', chart: 'heatmap', title: 'Status by month', series: [],
      heatRows: ['Sent', 'Overdue'],
      heatColumns: ['May', 'Jun'],
      cells: [
        { row: 'Sent', column: 'May', value: 40, formatted: '40', step: 4 },
        { row: 'Sent', column: 'Jun', value: 80, formatted: '80', step: 5 },
        { row: 'Overdue', column: 'May', value: 4, formatted: '4', step: 0 },
        // The pair nothing matched, which must not look like a small number.
        { row: 'Overdue', column: 'Jun', value: 0, formatted: '0', step: 0, empty: true },
      ],
    },
    {
      kind: 'chart', chart: 'gauge', title: 'Against plan', series: [],
      gauge: {
        value: 49900000, formatted: '€49.9M', target: 40000000,
        targetFormatted: '€40.0M', targetLabel: 'Plan', share: 1, over: '25% over',
      },
    },
    {
      kind: 'chart', chart: 'treemap', title: 'Territories', series: [],
      rects: [
        { label: 'North', value: 100, formatted: '€100k', x: 0, y: 0, w: 0.6, h: 1, slot: 0, depth: 0 },
        { label: 'Aurora', value: 70, formatted: '€70k', x: 0, y: 0, w: 0.6, h: 0.7, group: 'North', slot: 0, depth: 1 },
        { label: 'Baltic', value: 30, formatted: '€30k', x: 0, y: 0.7, w: 0.6, h: 0.3, group: 'North', slot: 0, depth: 1 },
        { label: 'South', value: 60, formatted: '€60k', x: 0.6, y: 0, w: 0.4, h: 1, slot: 1, depth: 0 },
        { label: 'Corvus', value: 60, formatted: '€60k', x: 0.6, y: 0, w: 0.4, h: 1, group: 'South', slot: 1, depth: 1 },
      ],
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
  if (req.url === '/real') {
    res.writeHead(200, { 'content-type': 'text/html' })
    return res.end(HOST.replace('report="monthly"', 'report="every-chart"'))
  }
  if (req.url.startsWith('/v1/embed/reports/every-chart')) {
    // The payload this server actually produces, written by
    // internal/app/run's TestPayloadForEveryChartType. Serving a hand-written
    // stub here would mean the two ends are tested against two different
    // fictions, and a renamed field leaves the product blank with both suites
    // green.
    res.writeHead(200, { 'content-type': 'application/json' })
    return res.end(readFileSync('testdata/payload.json'))
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
ok('renders every block', await report.locator('.panel').count() === 15)
ok('the headline value is shown', (await report.locator('.stat').first().innerText()) === '€49.9M')
ok('a delta is coloured by meaning, not direction',
  await report.locator('.delta b.up').first().isVisible())
ok('the table says what it is showing',
  (await report.locator('.panel', { hasText: 'Invoices' }).innerText()).includes('1 of 1284'))

/* -- The chart types ------------------------------------------------------ */
const stack = report.locator('.panel', { hasText: 'Billed by carrier' })
ok('a stacked bar draws a segment per series that has one',
  await stack.locator('.track.stack .fill').count() === 3)
ok('and the total beside it is the one the server formatted',
  (await stack.innerText()).includes('€8.0M'))
ok('two series get a legend, because colour alone is not a name',
  await stack.locator('.legend .key').count() === 2)
ok('series take different colours',
  await stack.locator('.fill').first().evaluate((a) => getComputedStyle(a).backgroundColor)
  !== await stack.locator('.fill').last().evaluate((b) => getComputedStyle(b).backgroundColor))

const trend = report.locator('.panel', { hasText: 'Trend' })
ok('a line chart draws a path', await trend.locator('path.line').count() === 1)
ok('and its axis carries the labels the engine formatted',
  (await trend.locator('.ys').innerText()).includes('€8.0M'))
ok('one series gets no legend — the title already names it',
  await trend.locator('.legend').count() === 0)

const donut = report.locator('.panel', { hasText: 'Share' })
ok('a donut draws an arc per slice', await donut.locator('.pie circle').count() === 2)
ok('and direct-labels them rather than needing a legend',
  (await donut.innerText()).includes('€7.0M'))

const bubbles = report.locator('.panel', { hasText: 'Paid against billed' })
ok('a bubble chart draws a dot per point', await bubbles.locator('circle.dot').count() === 2)
ok('sized by its measure, not all alike', await bubbles.locator('circle.dot').first()
  .evaluate((a, b) => Number(a.getAttribute('r')) > Number(b.getAttribute('r')),
    await bubbles.locator('circle.dot').last().elementHandle()))

/* -- Combo, funnel, waterfall --------------------------------------------- */
const combo = report.locator('.panel', { hasText: 'Billed and margin' })
ok('a combo draws bars and a line together',
  await combo.locator('rect.col').count() === 2 && await combo.locator('path.line').count() === 1)
ok('a combo always has a legend — the mark shape says bar or line, not which measure',
  await combo.locator('.legend .key').count() === 2)
ok('a measure on its own scale says so, rather than looking comparable',
  (await combo.innerText()).includes('(right)') && (await combo.locator('.axis2').innerText()).includes('20%'))

const funnel = report.locator('.panel', { hasText: 'Conversion' })
ok('a funnel draws a band per stage', await funnel.locator('.band').count() === 3)
ok('and it narrows', await funnel.locator('.band').first().evaluate(
  (a, b) => a.getBoundingClientRect().width > b.getBoundingClientRect().width,
  await funnel.locator('.band').last().elementHandle()))
ok('the fall is shown between the stages, not hidden in a tooltip',
  (await funnel.innerText()).includes('-60.0%'))
ok('stages take the ordinal ramp, so the order is in the colour',
  await funnel.locator('.band').first().evaluate((a) => getComputedStyle(a).backgroundColor)
  !== await funnel.locator('.band').last().evaluate((b) => getComputedStyle(b).backgroundColor))

const fall = report.locator('.panel', { hasText: 'Movement' })
ok('a waterfall draws a bar per step and a closing total',
  await fall.locator('rect.col').count() === 4)
ok('a fall is coloured against a rise',
  await fall.locator('rect.col').nth(1).getAttribute('fill')
  !== await fall.locator('rect.col').nth(2).getAttribute('fill'))
ok('and the total is neither',
  await fall.locator('rect.col').last().getAttribute('fill') === 'var(--cr-neutral)')

/* -- Heatmap, gauge, treemap ---------------------------------------------- */
const heat = report.locator('.panel', { hasText: 'Status by month' })
ok('a heatmap draws every pair', await heat.locator('.cell').count() === 4)
ok('a pair nothing matched is drawn as absence, not as a small number',
  await heat.locator('.cell.none').count() === 1)

const gauge = report.locator('.panel', { hasText: 'Against plan' })
ok('a gauge draws its arc over a track', await gauge.locator('.gauge circle').count() === 2)
ok('beating the target is said in words, because the arc is capped',
  (await gauge.innerText()).includes('25% over'))
ok('and the figures are the accessible reading of a decorative arc',
  (await gauge.getAttribute('aria-label') ?? '').includes('€40.0M'))

const tree = report.locator('.panel', { hasText: 'Territories' })
ok('a treemap places every leaf', await tree.locator('.tree-cell').count() === 3)
ok('grouped leaves sit inside a named frame',
  await tree.locator('.tree-frame').count() === 2
  // textContent, not innerText: the group label is upper-cased in CSS, and
  // asserting on the rendered string would make this a test of the styling.
  && (await tree.locator('.tree-group').first().textContent()) === 'North')
ok('a leaf takes its group colour, so what belongs together looks it',
  await tree.locator('.tree-cell').first().evaluate((a) => getComputedStyle(a).backgroundColor)
  === await tree.locator('.tree-cell').nth(1).evaluate((b) => getComputedStyle(b).backgroundColor))

/* -- The map -------------------------------------------------------------- */
const map = report.locator('.panel', { hasText: 'Depots' })
ok('a choropleth draws its projected polygons', await map.locator('.shapes path').count() === 1)
ok('a heat layer spreads the points', await map.locator('.heat circle').count() === 2)
ok('a bubble layer places them', await map.locator('.dots circle.pin').count() === 2)
ok('a flow layer draws an arc between two places',
  await map.locator('.flows path').count() === 1)
ok('a shaded map carries the legend that makes its colours readable',
  await map.locator('.legend.ramp .key').count() === 1)
ok('a basemap credits its tile source, which every one of them requires',
  (await map.locator('.credit').innerText()).includes('OpenStreetMap'))
ok('tiles are laid under the data, not over it',
  await map.locator('.tiles img').count() > 0)
ok('and they leak no referrer to the tile server',
  await map.locator('.tiles img').first().getAttribute('referrerpolicy') === 'no-referrer')

/* A mark that says nothing when pointed at reads as broken. */
await map.locator('.shapes path').first().hover()
ok('a mark answers when hovered',
  (await map.locator('.tip').innerText()).includes('1,500'))

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

/* -- The filter bar -------------------------------------------------------- */
ok('the report draws its own filter bar', await report.locator('.filters .filter').count() === 2)
ok('an enum lists the values the server sent',
  await report.locator('.filters select option').count() === 4)
ok('a date filter offers both ends of a range',
  await report.locator('.filters input[type=date]').count() === 2)

const beforeBar = requests
await report.locator('.filters select').selectOption('overdue')
await page.waitForFunction(() => document.querySelector('#r').shadowRoot
  .querySelector('.stat')?.textContent === '€12.1M')
ok('using the bar refetches', requests > beforeBar)
ok('and sends the operator the server compiles, not a value it has to guess',
  lastBody.includes('"op":"in"') && lastBody.includes('overdue'))
ok('the control reflects what is applied after the reload',
  await report.locator('.filters select').inputValue() === 'overdue')

// A failed request must not take the controls with it: a filter that matched
// nothing would otherwise be unrecoverable.
unauthorized = true
await report.locator('.filters select').selectOption('paid')
await report.locator('.msg.err').waitFor()
ok('an error leaves the bar in place to undo what caused it',
  await report.locator('.filters select').count() === 1)
unauthorized = false
await page.evaluate(() => { document.querySelector('#r').filters = {} })
await report.locator('.panel').first().waitFor()

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

/* -- The payload the server really sends ---------------------------------- */

/*
 * Everything above this point is the viewer against stubs written by hand in
 * this file. That proves the viewer draws a shape; it does not prove the shape
 * is the one cronos sends. This does: the fixture is written by the Go tests
 * from a real render against a real database, so a field renamed on one side
 * fails here rather than in somebody's dashboard.
 */
const real = await browser.newPage()
const realErrors = []
real.on('pageerror', (e) => realErrors.push(String(e)))
real.on('console', (m) => m.type() === 'error' && realErrors.push(m.text()))
await real.goto(`${base}/real`, { waitUntil: 'domcontentloaded' })
await real.evaluate((b) => document.querySelector('#r').setAttribute('endpoint', b), base)
const live = real.locator('#r')
await live.locator('.panel').first().waitFor()

ok('every block of a real render draws', await live.locator('.panel').count() === 17)
ok('and none of them fell through to "needs a newer viewer"',
  await live.locator('.unaffected', { hasText: 'newer viewer' }).count() === 0)
ok('nothing was thrown drawing it', realErrors.length === 0)

// The marks each type owns, against the server's own numbers rather than a
// stub's. An empty panel is what a missing payload key looks like, and an
// empty panel looks like a report that matched no rows.
for (const [title, selector] of [
  ['Parcels by region', '.fill'],
  ['Stacked by carrier', '.track.stack .fill'],
  ['Line', 'path.line'],
  ['Area', 'path.area'],
  ['Pie', '.pie circle'],
  ['Donut', '.pie circle'],
  ['Scatter', 'circle.dot'],
  ['Bubble', 'circle.dot'],
  ['Map', '.shapes path'],
  ['Combo', 'rect.col'],
  ['Funnel', '.band'],
  ['Waterfall', 'rect.col'],
  ['Heatmap', '.cell'],
  ['Gauge', '.gauge circle'],
  ['Treemap', '.tree-cell'],
]) {
  const panel = live.locator('.panel').filter({ hasText: new RegExp(`^${title}`) })
  ok(`${title} draws its marks from the server's payload`,
    await panel.locator(selector).count() > 0)
}

ok('the filter bar is built from the report definition',
  await live.locator('.filters .filter').count() === 2)
ok('and an enum offers the values the definition listed',
  await live.locator('.filters select option').count() === 3)

await real.close()

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
