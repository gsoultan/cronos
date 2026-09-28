/*
 * Drives <cronos-report> in a real browser against a stub API.
 *
 * A host page is the thing being tested, not the component in isolation: the
 * claims worth checking are about what survives contact with someone else's
 * CSS, someone else's data, and someone else's expired token.
 */
import { createServer } from 'node:http'
import { readFileSync } from 'node:fs'
import { chromium, firefox, webkit } from 'playwright'

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
        tiles: {
          url: `${'{z}'}/${'{x}'}/${'{y}'}.png`, attribution: '© OpenStreetMap contributors', maxZoom: 19,
          tileSize: 256,
          credits: [{ text: '© OpenStreetMap contributors', href: 'https://www.openstreetmap.org/copyright' }],
          logo: "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='20' height='10'/%3E",
          logoAlt: 'Provider',
        },
      },
    },
    {
      // Six drops a street apart and one across the county: one circle with
      // a count at the first zoom, which a click has to take apart.
      kind: 'chart', chart: 'map', title: 'Nearby drops', series: [],
      map: {
        bounds: { minX: 0.4990, minY: 0.3320, maxX: 0.5010, maxY: 0.3330 },
        layers: ['cluster'],
        shapes: [], arcs: [], legend: [],
        markers: [0, 1, 2, 3, 4, 5].map((i) => ({
          label: `Drop ${i + 1}`, x: 0.49920 + i * 0.000004, y: 0.33250 + (i % 2) * 0.000004,
          value: 1, formatted: '1', weight: 1, slot: i % 2,
        })).concat([{ label: 'Far drop', x: 0.5008, y: 0.3322, value: 1, formatted: '1', weight: 1, slot: 0 }]),
        keys: [{ label: 'Northline', slot: 0 }, { label: 'Swift Parcel', slot: 1 }],
        note: 'Mapbox is not set up on this server, so the map is drawn without its basemap.',
        partial: 'This map shows the first 5,000 places of more — narrow the filters to see the rest.',
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

/* A large map: a million places, sent as the cells the server gathers them
   into — here a grid of 1,200, in two categories — with the way to ask for more.
   Generated rather than written by the server: a real one is hundreds of
   kilobytes of cells, and the Go tests already pin the field names. */
const LARGE = { minX: 0.5125, minY: 0.3285, maxX: 0.5145, maxY: 0.3300 }
const largeMap = () => {
  const x = [], y = [], n = [], v = [], s = [], l = []
  for (let i = 0; i < 40; i++) {
    for (let j = 0; j < 30; j++) {
      x.push(LARGE.minX + (i + 0.5) * (LARGE.maxX - LARGE.minX) / 40)
      y.push(LARGE.minY + (j + 0.5) * (LARGE.maxY - LARGE.minY) / 30)
      n.push(1 + ((i * 7 + j * 13) % 900))
      v.push(10 + ((i * 3 + j) % 50))
      s.push((i + j) % 2)
      l.push('')
    }
  }
  return {
    title: 'A million drops',
    blocks: [{
      kind: 'chart', chart: 'map', title: 'Every drop', series: [],
      map: {
        bounds: LARGE, layers: ['scatter'], shapes: [], markers: [], arcs: [], legend: [],
        keys: [{ label: 'Aurora', slot: 0 }, { label: 'Baltic', slot: 1 }],
        cells: { size: 0.00005, x, y, n, v, s, l },
        places: 1_000_000,
        detail: { output: 'screen', block: 0, categories: ['Aurora', 'Baltic'] },
      },
    }],
  }
}
/* A view: one named place at the middle of whatever was asked for, and a few
   around it — so the check can point at the middle and read its name. */
const largeView = (ask) => {
  const [x0, y0, x1, y1] = ask.view
  const cx = (x0 + x1) / 2, cy = (y0 + y1) / 2
  const x = [cx], y = [cy], n = [1], v = [42], s = [0], l = ['Centre drop']
  for (let i = 1; i <= 8; i++) {
    // Along the top, clear of the middle.
    x.push(x0 + (x1 - x0) * i / 10); y.push(y0 + (y1 - y0) * 0.15)
    n.push(1); v.push(i); s.push(i % 2); l.push(`Drop ${i}`)
  }
  return {
    bounds: { minX: x0, minY: y0, maxX: x1, maxY: y1 }, layers: ['scatter'],
    shapes: [], markers: [], arcs: [], legend: [],
    cells: { size: (x1 - x0) / 400, x, y, n, v, s, l },
  }
}
const viewAsks = []
/* Views are answered once the map as it opened has been looked at. A view is
   a few places where the opening was a thousand cells, and whether one had
   landed by the time the canvas was read was up to the engine: Chrome was
   slower to ask than Firefox was to answer. */
let answerViews
const viewsAnswered = new Promise((r) => { answerViews = r })

/* A map that sets the report's filters — a region a click picks, the area its
   view sets — with a second dataset drawn over it. The answer echoes what was
   asked for, as the server's does, so the check can see the map mark what is
   applied. */
const pickBodies = []
/* A diverging map: a fall and a rise either side of zero, in two hues. */
const changes = (world) => ({
  kind: 'chart', chart: 'map', title: 'Change on last month', series: [],
  map: {
    bounds: world, layers: ['polygon'], markers: [], arcs: [], ramp: 'diverging',
    legend: [{ step: 2, from: '-8', to: '0' }, { step: 3, from: '0', to: '12' }],
    shapes: [
      { label: 'Down', path: 'M0.49 0.32L0.495 0.32L0.495 0.34L0.49 0.34Z', value: -8, formatted: '-8', step: 2 },
      { label: 'Up', path: 'M0.495 0.32L0.5 0.32L0.5 0.34L0.495 0.34Z', value: 12, formatted: '12', step: 3 },
    ],
  },
})
/* Two depots, a lorry between them and the ground each serves — named on the
   map, the lorry moving, and a basemap that follows the page's theme. */
const catchments = (world) => ({
  kind: 'chart', chart: 'map', title: 'Catchments', series: [],
  map: {
    bounds: world, layers: ['radius', 'scatter', 'flow'], shapes: [], legend: [],
    radiusKm: 25, labels: true, animate: true,
    markers: [
      { label: 'West depot', x: 0.492, y: 0.33, value: 4, formatted: '4', weight: 1 },
      { label: 'East depot', x: 0.498, y: 0.33, value: 2, formatted: '2', weight: 0.5 },
    ],
    arcs: [{ label: 'West → East', x1: 0.492, y1: 0.33, x2: 0.498, y2: 0.33, value: 9, formatted: '9', weight: 1 }],
    tiles: {
      url: 'L/{z}/{x}/{y}.png', attribution: '© Light', maxZoom: 19, tileSize: 256,
      dark: { url: 'D/{z}/{x}/{y}.png', attribution: '© Dark', maxZoom: 19, tileSize: 256 },
    },
  },
})
const pickMap = (body) => {
  const sent = JSON.parse(body || '{}').filters ?? {}
  const world = { minX: 0.49, minY: 0.32, maxX: 0.5, maxY: 0.34 }
  return {
    title: 'Map filters',
    blocks: [changes(world), catchments(world), {
      kind: 'chart', chart: 'map', title: 'Regions', series: [],
      map: {
        bounds: world, layers: ['polygon'], markers: [], arcs: [],
        legend: [{ step: 1, from: '300', to: '300' }, { step: 5, from: '1,500', to: '1,500' }],
        shapes: [
          { label: 'England', path: 'M0.49 0.32L0.495 0.32L0.495 0.34L0.49 0.34Z', value: 1500, formatted: '1,500', step: 5 },
          { label: 'Wales', path: 'M0.495 0.32L0.5 0.32L0.5 0.34L0.495 0.34Z', value: 300, formatted: '300', step: 1 },
        ],
        pick: { filter: 'region', values: sent.region?.values ?? [] },
        area: sent.where ? { filter: 'where', op: sent.where.op, values: sent.where.values } : { filter: 'where' },
        overlays: [{
          title: 'Warehouses', layers: ['scatter'], bounds: world, shapes: [], arcs: [], legend: [],
          markers: [{ label: 'Hub', x: 0.4975, y: 0.325, value: 1, formatted: '1', weight: 1 }],
        }],
      },
    }],
  }
}

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
  if (req.url === '/large') {
    res.writeHead(200, { 'content-type': 'text/html' })
    return res.end(HOST.replace('report="monthly"', 'report="large-map"'))
  }
  if (req.url === '/v1/embed/reports/large-map/map') {
    let body = ''
    req.on('data', (c) => (body += c))
    return req.on('end', async () => {
      const ask = JSON.parse(body)
      viewAsks.push(ask)
      await viewsAnswered
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end(JSON.stringify(largeView(ask)))
    })
  }
  if (req.url === '/picks') {
    res.writeHead(200, { 'content-type': 'text/html' })
    return res.end(HOST.replace('report="monthly"', 'report="map-filters"'))
  }
  if (req.url === '/v1/embed/reports/map-filters') {
    let body = ''
    req.on('data', (c) => (body += c))
    return req.on('end', () => {
      pickBodies.push(body)
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end(JSON.stringify(pickMap(body)))
    })
  }
  if (req.url === '/v1/embed/reports/large-map') {
    res.writeHead(200, { 'content-type': 'application/json' })
    return res.end(JSON.stringify(largeMap()))
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

/* Chrome by default, the browser a person would use; BROWSER=webkit or
   BROWSER=firefox runs every check below in Safari's engine or Firefox's —
   an ISV's customers use all three, and a shadow root, a pointer event or an
   adopted stylesheet is where engines still differ. */
const engine = process.env.BROWSER ?? 'chrome'
const browser = engine === 'chrome'
  ? await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
  : await { webkit, firefox, chromium }[engine].launch()
console.log(`  in ${engine}`)
const page = await browser.newPage()
let fails = 0
const ok = (name, cond) => { console.log(`  ${cond ? 'ok  ' : 'FAIL'} ${name}`); if (!cond) fails++ }

await page.goto(base, { waitUntil: 'domcontentloaded' })
await page.evaluate((b) => document.querySelector('#r').setAttribute('endpoint', b), base)
const report = page.locator('#r')
await report.locator('.panel').first().waitFor()

/* -- It renders ----------------------------------------------------------- */
ok('renders every block', await report.locator('.panel').count() === 16)
ok('the headline value is shown', (await report.locator('.stat').first().innerText()) === '€49.9M')
ok('a delta is coloured by meaning, not direction',
  await report.locator('.delta b.up').first().isVisible())
ok('the table says what it is showing',
  (await report.locator('.panel', { hasText: 'Invoices' }).innerText()).includes('1 of 1284'))

/* -- The chart types ------------------------------------------------------ */
// Marks by their part — the names a host styles them by — and colours by the
// fill a reader sees.
const fillOf = (mark) => mark.evaluate((a) => getComputedStyle(a).fill)
const textOf = async (panel) => (await panel.textContent()) ?? ''

const stack = report.locator('.panel', { hasText: 'Billed by carrier' })
ok('a stacked bar draws a segment per series that has one', await stack.locator('[part=bar]').count() === 3)
ok('and the total beside it is the one the server formatted', (await textOf(stack)).includes('€8.0M'))
ok('two series get a legend, because colour alone is not a name',
  await stack.locator('.legend .key').count() === 2)
ok('series take different colours',
  await fillOf(stack.locator('[part=bar]').first()) !== await fillOf(stack.locator('[part=bar]').last()))
await stack.locator('.legend .key').first().click()
const without = await stack.locator('[part=bar]').count()
await stack.locator('.legend .key').first().click()
ok('a key hides its series, and shows it again',
  without === 1 && await stack.locator('[part=bar]').count() === 3
  && await stack.locator('.legend .key').first().getAttribute('aria-pressed') === 'true')

const trend = report.locator('.panel', { hasText: 'Trend' })
ok('a line chart draws a path', await trend.locator('[part=line]').count() === 1)
ok('and its axis carries the labels the engine formatted',
  (await trend.locator('text.tick').allTextContents()).includes('€8.0M'))
ok('one series gets no legend — the title already names it',
  await trend.locator('.legend').count() === 0)

const donut = report.locator('.panel', { hasText: 'Share' })
ok('a donut draws a slice per share', await donut.locator('[part=slice]').count() === 2)
ok('and lists each with its value and its share',
  (await textOf(donut)).includes('€7.0M') && (await textOf(donut)).includes('%'))

const bubbles = report.locator('.panel', { hasText: 'Paid against billed' })
ok('a bubble chart draws a dot per point', await bubbles.locator('[part=dot]').count() === 2)
ok('sized by its measure, not all alike', await bubbles.locator('[part=dot]').first()
  .evaluate((a, b) => Number(a.getAttribute('r')) > Number(b.getAttribute('r')),
    await bubbles.locator('[part=dot]').last().elementHandle()))
// It was drawn in a square and letterboxed: the dots in a strip down the
// middle of a wide panel, under an axis that ran its whole width.
ok('and its dots are placed across the whole plot, not a strip of it',
  await bubbles.locator('svg.canvas').evaluate((svg) => {
    const box = svg.getBoundingClientRect()
    const xs = [...svg.querySelectorAll('.dot')].map((d) => d.getBoundingClientRect().x - box.x)
    return Math.max(...xs) - Math.min(...xs) > box.width * 0.3
  }))

/* -- Combo, funnel, waterfall --------------------------------------------- */
const combo = report.locator('.panel', { hasText: 'Billed and margin' })
ok('a combo draws bars and a line together',
  await combo.locator('[part=bar]').count() === 2 && await combo.locator('[part=line]').count() === 1)
ok('a combo always has a legend — the mark shape says bar or line, not which measure',
  await combo.locator('.legend .key').count() === 2)
// It was a sentence under the chart giving the range, and the line it scaled
// was drawn against nothing a reader could see.
ok('a measure on its own scale is read against an axis of its own, on the right',
  (await textOf(combo)).includes('(right)') && await combo.locator('svg.canvas').evaluate((svg) => {
    const box = svg.getBoundingClientRect()
    return [...svg.querySelectorAll('text.tick')].some((t) =>
      t.textContent === '20%' && t.getBoundingClientRect().x > box.x + box.width / 2)
  }))

const funnel = report.locator('.panel', { hasText: 'Conversion' })
ok('a funnel draws a band per stage', await funnel.locator('[part=stage]').count() === 3)
ok('and it narrows', await funnel.locator('[part=stage]').first().evaluate(
  (a, b) => a.getBoundingClientRect().width > b.getBoundingClientRect().width,
  await funnel.locator('[part=stage]').last().elementHandle()))
ok('the fall is shown between the stages, not hidden in a tooltip', (await textOf(funnel)).includes('-60.0%'))
ok('stages take the ordinal ramp, so the order is in the colour',
  await fillOf(funnel.locator('[part=stage]').first()) !== await fillOf(funnel.locator('[part=stage]').last()))

const fall = report.locator('.panel', { hasText: 'Movement' })
ok('a waterfall draws a column per step and a closing total', await fall.locator('[part=bar]').count() === 4)
ok('a fall is coloured against a rise',
  await fillOf(fall.locator('[part=bar]').nth(1)) !== await fillOf(fall.locator('[part=bar]').nth(2)))
ok('and the total is neither',
  (await fall.locator('[part=bar]').last().getAttribute('style') ?? '').includes('var(--cr-neutral)'))
ok('each column says its change over it, rather than in a tooltip',
  await fall.locator('text.value').count() === 4)

/* -- Heatmap, gauge, treemap ---------------------------------------------- */
const heat = report.locator('.panel', { hasText: 'Status by month' })
ok('a heatmap draws every pair', await heat.locator('[part=cell]').count() === 4)
ok('a pair nothing matched is drawn as absence, not as a small number',
  await heat.locator('.cell.none').count() === 1)
ok('a cell says its value, and the key says what each shade spans',
  await heat.locator('text.cell-value').count() === 3 && await heat.locator('.legend .key').count() > 0)

const gauge = report.locator('.panel', { hasText: 'Against plan' })
ok('a gauge draws its reading over a track',
  await gauge.locator('.gauge .track').count() === 1 && await gauge.locator('.gauge .reading').count() === 1)
ok('beating the target is said in words, because the arc is capped', (await textOf(gauge)).includes('25% over'))
ok('and the figures are the accessible reading of a decorative arc',
  (await gauge.getAttribute('aria-label') ?? '').includes('€40.0M'))

const tree = report.locator('.panel', { hasText: 'Territories' })
ok('a treemap places every leaf', await tree.locator('[part=cell]').count() === 3)
ok('grouped leaves sit under a named header',
  await tree.locator('.tree-frame').count() === 2
  // textContent, not innerText: the group label is upper-cased in CSS, and
  // asserting on the rendered string would make this a test of the styling.
  && (await tree.locator('.tree-group').first().textContent()) === 'North')
ok('a leaf takes its group colour, so what belongs together looks it',
  await fillOf(tree.locator('[part=cell]').first()) === await fillOf(tree.locator('[part=cell]').nth(1)))

/* -- Every chart ----------------------------------------------------------- */
// The report container is .grid and so, once, were the plot's gridlines: the
// rule that gave gridlines a stroke gave it to every shape in the report that
// did not set one, and the invisible hover target of a stretched plot drew as
// thick grey bars down its sides.
ok('no mark inherits a stroke it did not ask for',
  await report.locator('.hit, [part=bar], .cell:not(.none), .tree-cell').evaluateAll((marks) =>
    marks.every((m) => getComputedStyle(m).stroke === 'none')))
ok('every chart is drawn at its own size, not stretched to it',
  await report.locator('svg.canvas').evaluateAll((svgs) => svgs.length > 0 && svgs.every((s) =>
    Math.abs(s.viewBox.baseVal.width - s.getBoundingClientRect().width) < 1.5)))
// Four pixels of play: Firefox's box for a line of SVG text takes in each
// glyph's side bearings, three pixels a side at this size, where Chrome's and
// Safari's stop at the ink. A label that runs off — "Aug 2026" drawn as
// "Aug 202" — is off by far more than that.
const runOff = await report.locator('svg.canvas').evaluateAll((svgs) => svgs.flatMap((s) => {
  const box = s.getBoundingClientRect()
  return [...s.querySelectorAll('text')].filter((t) => {
    const r = t.getBoundingClientRect()
    return r.width > 0 && (r.left < box.left - 4 || r.right > box.right + 4)
  }).map((t) => t.textContent)
}))
ok(`and no label runs off the chart it is on${runOff.length ? ` (${runOff.join(', ')})` : ''}`, runOff.length === 0)

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
// Laid by the first draw, which follows the stage being sized — a rendering
// update after the panel is placed, and WebKit on Linux takes its time over
// one; the stage's own marks are there from the start.
await map.locator('.tiles img').first().waitFor({ timeout: 5000 }).catch(() => {})
ok('tiles are laid under the data, not over it',
  await map.locator('.tiles img').count() > 0)
// The origin and not the path. OpenStreetMap refuses a tile request with no
// Referer at all, and a Mapbox token restricted to a domain refuses one too —
// `no-referrer`, which this asserted, drew a basemap of refusal tiles. The path
// is our customer's application and still goes nowhere.
ok('and they tell the tile server which site asks, and nothing past it',
  await map.locator('.tiles img').first().getAttribute('referrerpolicy') === 'strict-origin')
ok("a provider's credit links where its terms want it to",
  await map.locator('.credit a').first().getAttribute('href') === 'https://www.openstreetmap.org/copyright'
  && (await map.locator('.credit a').first().getAttribute('rel')).includes('noopener'))
ok('and the logo its terms require sits on the map',
  await map.locator('.geo-stage .geo-logo').getAttribute('alt') === 'Provider')

/* Where a dot is drawn is where the data says it is. Every coordinate used to
   go through the chart helper that rounds to three decimals — a sub-pixel in a
   bar chart's units and forty kilometres in world units — so London (0.4996)
   was drawn at 0.5, and on a map of one city every dot shared a handful of
   places and every radius rounded to nothing. */
ok('a marker is placed where the server put it, not on a forty-kilometre grid',
  await map.locator('.dots circle').evaluateAll((cs) => cs.map((c) => c.getAttribute('cx')).sort().join())
  === '0.4928,0.4996')
ok('and it is drawn a size a reader can see',
  await map.locator('.dots circle').first().evaluate((c) => c.getBoundingClientRect().width) >= 8)

/* Stroke inherits, and the report grid sets one a unit wide — on a map, the
   width of the planet. A heat disc that did not refuse it drew a grey sheet
   over everything under it. */
ok('a heat disc inherits no stroke from the page around it',
  await map.locator('.heat circle').first().evaluate((c) => getComputedStyle(c).stroke) === 'none')
ok('a flow is drawn in its colour, not in the colour of the paper',
  await map.locator('.flows path').first().evaluate((f) => getComputedStyle(f).stroke) === 'rgb(42, 120, 214)')

/* Pan and zoom. The viewBox is the window onto the world, so each gesture is
   a number that has to move the right way. */
const geo = () => map.locator('svg.geo').getAttribute('viewBox').then((v) => v.split(' ').map(Number))
// The stub's template is {z}/{x}/{y}.png, so a tile's zoom is its first segment.
const levels = () => map.locator('.tiles img').evaluateAll((is) =>
  [...new Set(is.map((i) => Number(i.getAttribute('src').split('/')[0])))])
const fitted = await geo()
const level = Math.max(...await levels())
await map.getByRole('button', { name: 'Zoom in' }).click()
await page.waitForTimeout(100)
const zoomed = await geo()
ok('a zoom button halves the window', Math.abs(zoomed[2] - fitted[2] / 2) < 1e-9)
ok('and the tiles follow it a level down', (await levels()).includes(level + 1))
// On screen before the pointer goes to it: a box measured below the fold is
// somewhere the mouse cannot reach, and the drag never starts.
await map.locator('.geo-stage').scrollIntoViewIfNeeded()
const box = await map.locator('.geo-stage').boundingBox()
await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
await page.mouse.down()
await page.mouse.move(box.x + box.width / 2 - 60, box.y + box.height / 2 - 30, { steps: 6 })
await page.mouse.up()
// Settled once the reader stops, a moment after the last move.
for (let i = 0; i < 20 && JSON.stringify(await geo()) === JSON.stringify(zoomed); i++) await page.waitForTimeout(50)
const dragged = await geo()
const perPx = zoomed[2] / box.width
ok('a drag moves the map by the length of the drag',
  Math.abs(dragged[0] - zoomed[0] - 60 * perPx) < perPx * 2 && Math.abs(dragged[1] - zoomed[1] - 30 * perPx) < perPx * 2)
const wheeled = await geo()
await page.mouse.wheel(0, 120)
await page.waitForTimeout(100)
ok('a plain wheel scrolls the page and leaves the map where it was',
  JSON.stringify(await geo()) === JSON.stringify(wheeled))
await map.locator('.geo-stage').focus()
await page.keyboard.press('0')
await page.waitForTimeout(100)
ok('and 0 puts it back as it arrived', JSON.stringify(await geo()) === JSON.stringify(fitted))

/* A press let go outside the map, before it had the pointer, is a release the
   map never hears about. It kept the press, and the next hover across the map
   dragged it. */
const edge = await map.locator('.geo-stage').boundingBox()
await page.mouse.move(edge.x + 1, edge.y + edge.height / 2)
await page.mouse.down()
await page.mouse.move(edge.x - 2, edge.y + edge.height / 2)
await page.mouse.up()
await page.mouse.move(edge.x + edge.width / 2, edge.y + edge.height / 3, { steps: 6 })
await page.waitForTimeout(100)
ok('a press let go outside the map leaves no drag behind for the next hover',
  JSON.stringify(await geo()) === JSON.stringify(fitted))

/* Clusters: a count where points would overlap, and a click that separates
   them. */
const nearby = report.locator('.panel', { hasText: 'Nearby drops' })
await nearby.locator('.cluster').first().waitFor()
ok('points a street apart are one circle with their count',
  await nearby.locator('.cluster').count() === 1 && (await nearby.locator('.cluster').innerText()) === '6')
ok('and the point across the county stands alone', await nearby.locator('svg.geo .pin').count() === 1)
await nearby.locator('.cluster').click()
await page.waitForTimeout(200)
ok('a click on the count zooms in until they separate',
  await nearby.locator('.cluster').count() === 0 && await nearby.locator('svg.geo .pin').count() >= 6)
ok('coloured by the category the legend names',
  await nearby.locator('.legend .key').count() === 2)
/* Colour alone leaves a reader who cannot tell two hues apart with a map of
   identical dots. Each category is drawn in a shape of its own too, and the
   key shows it. */
ok('and shaped by it, on the map and in the key',
  await nearby.locator('svg.geo circle.pin').count() >= 1 && await nearby.locator('svg.geo rect.pin').count() >= 1
  && await nearby.locator('.legend .swatch.circle').count() === 1
  && await nearby.locator('.legend .swatch.square').count() === 1)
ok('and a basemap that could not be drawn says why, under the map',
  (await nearby.locator('.credit .note').innerText()).includes('not set up'))
ok('a map drawn from part of its data says so',
  (await nearby.locator('[part=partial]').innerText()).includes('first 5,000'))

/* A bubble's size is a value, and says which: rings drawn to the bubbles'
   own rule, at round numbers up to the biggest on the map (1,200 here). */
ok('a bubble map keys its sizes',
  JSON.stringify(await map.locator('.legend.sizes .key').allInnerTexts()) === '["1,000","200","50"]')

/* A mark that says nothing when pointed at reads as broken. */
await map.locator('.shapes path').first().hover()
ok('a mark answers when hovered',
  (await map.locator('.tip').innerText()).includes('1,500'))

/* -- Fingers --------------------------------------------------------------- */

/*
 * On a phone one finger scrolls the page past the map and two move it: a map
 * that took one finger would trap somebody scrolling down our customer's page
 * inside it. Driven through Chrome's own input pipeline, as fingers arrive
 * from a screen. The other engines' drivers have no second finger to put
 * down, so this runs in Chrome's.
 */
if (engine === 'chrome' || engine === 'chromium') {
  const phone = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true })
  const hand = await phone.newPage()
  await hand.goto(base, { waitUntil: 'domcontentloaded' })
  await hand.evaluate((b) => document.querySelector('#r').setAttribute('endpoint', b), base)
  const touched = hand.locator('#r').locator('.panel', { hasText: 'Depots' })
  await touched.locator('svg.geo').waitFor()
  await touched.locator('.geo-stage').scrollIntoViewIfNeeded()
  const seen = () => touched.locator('svg.geo').getAttribute('viewBox').then((v) => v.split(' ').map(Number))
  const cdp = await phone.newCDPSession(hand)
  // Each gesture from wherever the map is now: one finger may have scrolled it.
  const gesture = async (from, to) => {
    const at = await touched.locator('.geo-stage').boundingBox()
    const place = (points) => points.map(([x, y], id) => ({ x: at.x + at.width / 2 + x, y: at.y + at.height / 2 + y, id }))
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: place(from) })
    for (let i = 1; i <= 6; i++) {
      const step = from.map(([x, y], k) => [x + (to[k][0] - x) * i / 6, y + (to[k][1] - y) * i / 6])
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: place(step) })
    }
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
    await hand.waitForTimeout(150)
  }

  const still = await seen()
  await gesture([[0, 0]], [[0, -90]])
  ok('one finger leaves the map where it was', JSON.stringify(await seen()) === JSON.stringify(still))
  ok('and says two are what move it', (await touched.locator('.geo-hint').innerText()).includes('two fingers'))

  await gesture([[-30, 0], [30, 0]], [[-90, 0], [90, 0]])
  const pinched = await seen()
  ok('two fingers spread apart zoom it in', pinched[2] < still[2] * 0.6)

  await gesture([[-40, 0], [40, 0]], [[-100, -30], [-20, -30]])
  const moved = await seen()
  ok('and moved together, move it the way they went',
    moved[0] > pinched[0] && moved[1] > pinched[1] && Math.abs(moved[2] - pinched[2]) < pinched[2] * 0.01)
  await phone.close()
}

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
  await report.locator('[part=bar]').first().evaluate((f) => getComputedStyle(f).fill) === 'rgb(255, 0, 0)')

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

ok('every block of a real render draws', await live.locator('.panel').count() === 29)
ok('and none of them fell through to "needs a newer viewer"',
  await live.locator('.unaffected', { hasText: 'newer viewer' }).count() === 0)
ok('nothing was thrown drawing it', realErrors.length === 0)

// The marks each type owns, against the server's own numbers rather than a
// stub's. An empty panel is what a missing payload key looks like, and an
// empty panel looks like a report that matched no rows.
for (const [title, selector] of [
  ['Parcels by region', '[part=bar]'],
  ['Stacked by carrier', '[part=bar]'],
  ['Line', '[part=line]'],
  ['Area', '[part=area]'],
  ['Pie', '[part=slice]'],
  ['Donut', '[part=slice]'],
  ['Scatter', '[part=dot]'],
  ['Bubble', '[part=dot]'],
  ['Map', '.shapes path'],
  ['Routes', '.routes path.route'],
  ['Hexagons', '.hexes path'],
  ['Clusters', 'circle.pin, .cluster'],
  ['By day', '.shapes path'],
  ['Combo', '[part=bar]'],
  ['Funnel', '[part=stage]'],
  ['Waterfall', '[part=bar]'],
  ['Heatmap', '[part=cell]'],
  ['Gauge', '.gauge .track'],
  ['Treemap', '[part=cell]'],
  ['Columns by carrier', '[part=bar]'],
  ['Share by carrier', '[part=bar]'],
  ['Radar', '[part=point]'],
  ['Bullet', '[part=target]'],
  ['Histogram', '[part=bar]'],
  ['Box plot', '[part=box]'],
  ['Sankey', '[part=flow]'],
  ['Sunburst', '[part=slice]'],
]) {
  const panel = live.locator('.panel').filter({ hasText: new RegExp(`^${title}`) })
  ok(`${title} draws its marks from the server's payload`,
    await panel.locator(selector).count() > 0)
}
const panelOf = (title) => live.locator('.panel').filter({ hasText: new RegExp(`^${title}`) })
// Grouped columns leave a pair the server padded with nothing undrawn: three
// columns for the three carrier-regions that have parcels, not four.
ok('grouped columns draw a column per pair that has one',
  await panelOf('Columns by carrier').locator('[part=bar]').count() === 3)
ok('a stack of shares is read against percentages',
  await panelOf('Share by carrier').locator('text.tick', { hasText: '100%' }).count() === 1)
{
  // Every part of a stack of shares reaches the top of its scale together:
  // England's two parts and Scotland's one each end at 100%.
  const tops = await panelOf('Share by carrier').locator('[part=bar]').evaluateAll((parts) =>
    parts.map((p) => p.getBoundingClientRect().top))
  ok('each bucket of a stack of shares fills to its whole',
    tops.length === 3 && Math.abs(Math.min(tops[0], tops[1]) - tops[2]) < 2)
}
ok('a radar draws a point per spoke per series, and names its spokes',
  await panelOf('Radar').locator('[part=point]').count() === 6 &&
  await panelOf('Radar').locator('text.name', { hasText: 'Edinburgh' }).count() === 1)
// A ring is a closed path, which fills black unless a rule says otherwise —
// and `.ring` is a map's radius, washed in a category's hue.
ok("a radar's rings are drawn and not filled",
  await panelOf('Radar').locator('path.web-ring').evaluateAll((rings) =>
    rings.length === 3 && rings.every((r) => getComputedStyle(r).fill === 'none')))
ok('a bullet draws a bar and a target mark per category, and says the share',
  await panelOf('Bullet').locator('[part=bar]').count() === 2 &&
  await panelOf('Bullet').locator('[part=target]').count() === 2 &&
  await panelOf('Bullet').locator('text.name', { hasText: '150%' }).count() === 1)
// Three depots in three of nine bins: an empty bin is a gap in the range, not
// a sliver that reads as a small count.
ok('a histogram draws a column per bin that has rows in it',
  await panelOf('Histogram').locator('[part=bar]').count() === 3 &&
  await panelOf('Histogram').locator('text.tick', { hasText: '1,200' }).count() === 1)
ok('a box plot draws a box, a median and whiskers per category',
  await panelOf('Box plot').locator('[part=box]').count() === 2 &&
  await panelOf('Box plot').locator('.median').count() === 2 &&
  await panelOf('Box plot').locator('.whisker').count() === 8)
ok('a sankey draws a band per pair that flows, and a node each side of it',
  await panelOf('Sankey').locator('[part=flow]').count() === 3 &&
  await panelOf('Sankey').locator('[part=node]').count() === 4 &&
  await panelOf('Sankey').locator('text.name', { hasText: 'Aurora · 1,900' }).count() === 1)
ok("a sunburst draws each part within its whole, and the server's whole in its middle",
  await panelOf('Sunburst').locator('[part=slice]').count() === 5 &&
  (await panelOf('Sunburst').locator('text.centre').textContent()) === '2,200')
ok("a donut says the server's whole in its middle",
  (await live.locator('.panel').filter({ hasText: /^Donut/ }).locator('text.centre').textContent()) === '2,200')

// A map that plays through time, as the server sends it: a day is that day's
// places and shades, a region with nothing in it drawn empty and saying so,
// and all periods is the whole again.
const byDay = live.locator('.panel').filter({ hasText: /^By day/ })
const wholeMap = await byDay.locator('[part=marker]').count()
await byDay.getByRole('slider', { name: 'Period' }).fill('1')
ok('a map that plays through time shows a day of it',
  (await byDay.locator('[part=period]').innerText()) === '2 Aug'
  && await byDay.locator('[part=marker]').count() === 1)
ok('and a region with nothing that day drawn empty, and saying so',
  (await byDay.locator('.shapes path.void').getAttribute('aria-label')) === 'Scotland, Nothing in 2 Aug')
await byDay.getByRole('button', { name: 'All periods' }).click()
ok('and every period together again',
  await byDay.locator('[part=marker]').count() === wholeMap && await byDay.locator('.shapes path.void').count() === 0)

ok('the filter bar is built from the report definition',
  await live.locator('.filters .filter').count() === 2)
ok('and an enum offers the values the definition listed',
  await live.locator('.filters select option').count() === 3)

await real.close()

/* -- A large map ---------------------------------------------------------- */

/*
 * A million places cannot be elements. They arrive as cells, are painted on a
 * canvas, and the map asks the server for the part in view each time a reader
 * stops moving it — with the element's own token and filters, the renderer
 * never holding either. Pointing at a painted place names it, though there is
 * no element under the pointer to listen.
 */
const big = await browser.newPage()
const bigErrors = []
big.on('pageerror', (e) => bigErrors.push(String(e)))
await big.goto(`${base}/large`, { waitUntil: 'domcontentloaded' })
await big.evaluate((b) => document.querySelector('#r').setAttribute('endpoint', b), base)
const bigMap = big.locator('#r')
await bigMap.locator('[part=map-canvas]').waitFor()

const painted = () => big.evaluate(() => {
  const c = document.querySelector('#r').shadowRoot.querySelector('[part=map-canvas]')
  const d = c.getContext('2d').getImageData(0, 0, c.width, c.height).data
  let n = 0
  for (let i = 3; i < d.length; i += 4) if (d[i] > 0) n++
  return n
})
// Painted once the stage has a size, which is a frame or two after it is in
// the page: read before then, a canvas is empty in any engine.
for (let i = 0; i < 40 && await painted() === 0; i++) await big.waitForTimeout(50)
ok('a large map paints its cells rather than building an element each',
  await painted() > 500 && await bigMap.locator('[part=marker]').count() === 0)
answerViews()
ok('and says how many places it holds',
  (await bigMap.locator('[part=places]').innerText()).includes('1,000,000 places'))

for (let i = 0; i < 40 && viewAsks.length === 0; i++) await big.waitForTimeout(100)
const first = viewAsks[0]
ok('it asks for the part in view once it settles', viewAsks.length >= 1)
ok('with the view, its size, the map it is and the colours it opened with',
  first && first.block === 0 && first.output === 'screen' && first.view.length === 4
  && first.view.every((n) => n >= 0 && n <= 1) && first.width > 0 && first.height > 0
  && JSON.stringify(first.categories) === '["Aurora","Baltic"]')
ok('and with the filters the report was drawn with', first && typeof first.filters === 'object')

// The answer puts a named place at the middle of the view it was asked for.
await big.waitForTimeout(300)
const mapStage = bigMap.locator('[part=chart]').locator('..')
const mapBox = await mapStage.boundingBox()
await big.mouse.move(mapBox.x + mapBox.width / 2, mapBox.y + mapBox.height / 2)
await big.waitForTimeout(100)
ok('pointing at a painted place names it',
  (await bigMap.locator('[part=tooltip]').innerText().catch(() => '')).includes('Centre drop'))

const asked = viewAsks.length
await bigMap.getByRole('button', { name: 'Zoom in' }).click()
for (let i = 0; i < 40 && viewAsks.length === asked; i++) await big.waitForTimeout(100)
const deeper = viewAsks.at(-1)
ok('zooming in asks for a smaller part of the world',
  viewAsks.length > asked && deeper.view[2] - deeper.view[0] < first.view[2] - first.view[0])
ok('nothing was thrown drawing a large map', bigErrors.length === 0)
await big.close()

/* -- A map sets the report's filters -------------------------------------- */

/*
 * A click on a region narrows the report to it, and "Filter to this view" to
 * the part of the world in view. The element applies both, as it applies any
 * change of filters — the request is the report again, with them — and says
 * so to a host that keeps the filters itself. A dataset drawn over the map is
 * drawn, in its own colour, and named under it.
 */
const picks = await browser.newPage()
const pickErrors = []
picks.on('pageerror', (e) => pickErrors.push(String(e)))
await picks.goto(`${base}/picks`, { waitUntil: 'domcontentloaded' })
await picks.evaluate((b) => {
  const r = document.querySelector('#r')
  // A host on a dark page says so on the element; see the basemap below.
  r.setAttribute('data-theme', 'dark')
  window.__set = []
  r.addEventListener('cronos:filter', (e) => window.__set.push(e.detail))
  r.setAttribute('endpoint', b)
}, base)
const regions = picks.locator('#r').locator('.panel', { hasText: 'Regions' })
await regions.locator('.shapes path').first().waitFor()
const reloaded = async (n) => {
  for (let i = 0; i < 50 && pickBodies.length < n; i++) await picks.waitForTimeout(40)
  await regions.locator('.shapes path').first().waitFor()
}
const sentFilters = () => JSON.parse(pickBodies.at(-1) || '{}').filters ?? {}

ok('a region a click filters by says so to the pointer',
  await regions.locator('.shapes path.pickable').count() === 2)
let sent0 = pickBodies.length
await regions.locator('.shapes path').first().dispatchEvent('click')
await reloaded(sent0 + 1)
const sets = await picks.evaluate(() => window.__set)
ok('a click on a region sets the filter the map is bound to',
  sets.length === 1 && sets[0].name === 'region'
  && JSON.stringify(sets[0].value) === '{"op":"in","values":["England"]}')
ok('and the report is asked for again with it',
  JSON.stringify(sentFilters().region) === '{"op":"in","values":["England"]}')
ok('the region the report is narrowed to is marked on the map',
  await regions.locator('.shapes path.picked').count() === 1)

sent0 = pickBodies.length
await regions.locator('.shapes path.picked').dispatchEvent('click')
await reloaded(sent0 + 1)
ok('a click on the picked region lets it go',
  sentFilters().region === undefined && await regions.locator('.shapes path.picked').count() === 0)

sent0 = pickBodies.length
await regions.getByRole('button', { name: 'Filter to this view' }).click()
await reloaded(sent0 + 1)
const area = sentFilters().where
ok('"Filter to this view" narrows the report to the box in view',
  area?.op === 'within' && area.values.length === 4 && area.values.every(Number.isFinite)
  && area.values[0] < area.values[2] && area.values[1] < area.values[3])
ok('which is drawn on the map, with the way back',
  await regions.locator('[part=map-area]').count() === 1
  && await regions.getByRole('button', { name: 'Show everywhere' }).count() === 1)
sent0 = pickBodies.length
await regions.getByRole('button', { name: 'Show everywhere' }).click()
await reloaded(sent0 + 1)
ok('"Show everywhere" lets it go', sentFilters().where === undefined)

const hub = regions.locator('svg.geo-above circle.pin')
ok('a dataset drawn over the map is drawn, above anything the map paints', await hub.count() === 1)
// Against a pin of the map's own, in the same SVG: both are computed colours,
// so the comparison is the one a reader's eye makes.
const fills = await hub.evaluate((c) => {
  const own = document.createElementNS('http://www.w3.org/2000/svg', 'circle')
  own.setAttribute('class', 'pin')
  c.ownerSVGElement.append(own)
  const out = [getComputedStyle(c).fill, getComputedStyle(own).fill]
  own.remove()
  return out
})
ok('in a colour of its own, not the map\'s pin colour',
  fills[1].startsWith('rgb') && fills[0].startsWith('rgb') && fills[0] !== fills[1])
ok('and named under the map', (await regions.locator('[part~=overlay-key]').innerText()).includes('Warehouses'))
sent0 = pickBodies.length
await hub.dispatchEvent('click')
await picks.waitForTimeout(200)
ok('a click on a place drawn over the map sets nothing', pickBodies.length === sent0)
/* A diverging ramp is two hues, the palest either side of the middle: the
   fall in the cool one, the rise in the warm — in the shapes and the legend
   alike, measured against the theme's own values for them. */
const shaded = picks.locator('#r').locator('.panel', { hasText: 'Change on last month' })
const hues = await shaded.locator('.shapes path').evaluateAll((paths) => {
  const probe = (v) => {
    const i = document.createElement('i')
    i.style.color = `var(${v})`
    paths[0].closest('.panel').append(i)
    const c = getComputedStyle(i).color
    i.remove()
    return c
  }
  return {
    fills: paths.map((p) => getComputedStyle(p).fill),
    want: [probe('--cr-div-3'), probe('--cr-div-4')],
    ramp: probe('--cr-ramp-3'),
  }
})
ok('a diverging map shades a fall and a rise in two hues',
  JSON.stringify(hues.fills) === JSON.stringify(hues.want) && hues.fills[0] !== hues.ramp)
ok('and its legend says which is which, in the same two',
  JSON.stringify(await shaded.locator('.legend.ramp .swatch').evaluateAll((s) =>
    s.map((i) => getComputedStyle(i).backgroundColor))) === JSON.stringify(hues.want))
/* A flow says which way it goes, a radius is the ground around each place,
   and the places are named on the map — none over another. */
const reach = picks.locator('#r').locator('.panel', { hasText: 'Catchments' })
await reach.locator('[part=flow-head]').first().waitFor()
ok('a flow ends in a head where it lands', await reach.locator('[part=flow-head]').count() === 1)
ok('a radius is drawn around each place', await reach.locator('[part=radius]').count() === 2)
const boxes = await reach.locator('.geo-label').evaluateAll((ls) => ls.map((l) => {
  const b = l.getBoundingClientRect()
  return [b.left, b.top, b.right, b.bottom, l.textContent]
}))
ok('the places are named on the map', boxes.length === 2 && boxes.some((b) => b[4] === 'West depot'))
ok('and no name sits on another', boxes.every((a, i) => boxes.every((b, j) =>
  i === j || a[2] <= b[0] || b[2] <= a[0] || a[3] <= b[1] || b[3] <= a[1])))
ok('a moving flow moves',
  await reach.locator('.flow').first().evaluate((f) => getComputedStyle(f).animationName) === 'cr-flow')
await picks.emulateMedia({ reducedMotion: 'reduce' })
ok('and holds still for a reader who asked for less motion',
  await reach.locator('.flow').first().evaluate((f) => getComputedStyle(f).animationName) === 'none')
await picks.emulateMedia({ reducedMotion: 'no-preference' })

/* A basemap that follows the page draws the provider's dark tiles on a dark
   page, and its light ones once the page is light again. */
const srcs = () => reach.locator('.tiles img').evaluateAll((is) => [...new Set(is.map((i) => i.getAttribute('src')[0]))])
ok('a basemap on a dark page is the dark one', JSON.stringify(await srcs()) === '["D"]')
await picks.evaluate(() => document.querySelector('#r').setAttribute('data-theme', 'light'))
await reach.getByRole('button', { name: 'Zoom in', exact: true }).click()
await picks.waitForTimeout(300)
ok('and the light one once the page is light', JSON.stringify(await srcs()) === '["L"]')
ok('nothing was thrown by a map that filters', pickErrors.length === 0)
await picks.close()

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
