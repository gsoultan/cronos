/*
 * What cronos does under load, measured rather than assumed.
 *
 * Three questions, in the order they cost money when the answer is bad:
 *
 *   1. Does a report get slower when several people open it at once, and by
 *      how much? A reporting tool that serialises is one where the second
 *      person waits for the first, and nobody reports it as a bug — they just
 *      stop using it.
 *   2. How fast does a burst go, and what is it waiting on? Five thousand
 *      statements at 06:00 either finish before anybody is awake or they do
 *      not.
 *   3. Does anything leak or degrade across the run — connections, memory, a
 *      pool that never returns to idle.
 *
 * No thresholds and no pass/fail. A benchmark that fails a build teaches
 * people to raise the number in the benchmark.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'

const API = process.env.API
const TOKEN = process.env.TOKEN
const ADMIN = process.env.ADMIN
const CUSTOMERS = Number(process.env.CUSTOMERS)
const REQUESTS = Number(process.env.REQUESTS)

/* One token per virtual reader. Driving one token at sixty-four in flight is
   one reader in a loop, which is what the render limit exists to stop — the
   first run of this measured the limiter and reported it as the server. */
const TOKENS = readFileSync(process.env.TOKENS, 'utf8').split('\n').filter(Boolean)

const row = (label, ...cells) => console.log('  ' + label.padEnd(34) + cells.join('   '))

/** Percentiles, from an unsorted list of milliseconds. */
function summarise(times) {
  if (times.length === 0) {
    // Every request failed. Reporting that is the result; crashing on an
    // empty percentile is the harness losing the finding it just made.
    return { n: 0, p50: 0, p95: 0, p99: 0, min: 0, max: 0, mean: 0 }
  }
  const sorted = times.toSorted((a, b) => a - b)
  const at = (p) => sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * p))]
  return {
    n: sorted.length,
    p50: at(0.5), p95: at(0.95), p99: at(0.99),
    min: sorted[0], max: sorted.at(-1),
    mean: sorted.reduce((a, b) => a + b, 0) / sorted.length,
  }
}

const ms = (n) => `${n.toFixed(1)}ms`.padStart(9)

/*
 * Which reader a request comes from, per request rather than per worker.
 *
 * The render limit is per bearer, so a worker that keeps one token sends every
 * one of its requests as the same reader — and at concurrency 1 that is the
 * whole run through a single bucket. More than half of an earlier run came back
 * 429, which made the numbers below a measurement of the rate limiter rather
 * than of the renderer, and the summary still concluded "it scales with
 * concurrency" from data that was 57% refusals.
 *
 * Spreading per request is also the truer shape: load on a deployment comes
 * from many readers, and one token hammering is exactly what the limiter is
 * there to stop.
 */
let issued = 0
const nextReader = () => TOKENS[issued++ % TOKENS.length]

async function render(report, reader = 0) {
  const started = performance.now()
  const r = await fetch(`${API}/v1/reports/${report}`, {
    method: 'POST',
    headers: {
      authorization: `Bearer ${nextReader()}`,
      'content-type': 'application/json',
    },
    body: '{}',
  })
  await r.arrayBuffer() // drained, so the timing includes reading the answer
  return { ms: performance.now() - started, ok: r.ok, status: r.status }
}

/** Runs `total` requests with at most `concurrency` in flight. */
async function drive(report, total, concurrency) {
  const times = []
  const failures = []
  let next = 0

  const worker = async (reader) => {
    for (;;) {
      const i = next++
      if (i >= total) return
      const result = await render(report, reader)
      if (result.ok) times.push(result.ms)
      else failures.push(result.status)
    }
  }

  const started = performance.now()
  await Promise.all(Array.from({ length: concurrency }, (_, i) => worker(i)))
  const wall = performance.now() - started

  return { ...summarise(times), failures, wall, throughput: (total / wall) * 1000 }
}

console.log(`\n=== rendering, ${CUSTOMERS} customers, ${CUSTOMERS * 5} invoices ===\n`)
row('concurrency', 'p50'.padStart(9), 'p95'.padStart(9), 'p99'.padStart(9),
  'max'.padStart(9), 'req/s'.padStart(8), 'failed '.padEnd(7), 'warehouse')

/* Warm first. The first render of a report compiles it, opens a connection and
   fills a page cache, and folding that into the numbers measures the startup
   rather than the steady state. */
await drive('billing-summary', 10, 1)

/* How many connections cronos holds to the warehouse while this is running.
   The pool's own comment says it is bounded because somebody else operates
   that database — this is what checks whether it actually is. */
async function warehouseConnections() {
  if (!process.env.PGCOUNT) return null
  const { execFileSync } = await import('node:child_process')
  try {
    return Number(execFileSync('podman', [
      'exec', 'cronos-pg', 'psql', '-tAqU', 'cronos', '-d', 'cronos', '-c',
      "SELECT count(*) FROM pg_stat_activity WHERE datname='cronos' AND application_name <> 'psql'",
    ]).toString().trim())
  } catch {
    return null
  }
}

const byConcurrency = {}
for (const concurrency of [1, 4, 16, 64]) {
  /* Sampled slowly, and never overlapping.
     
     At fifty milliseconds this spawned a psql per sample, each costing more
     than that, and they piled up until the harness was the load: p50 at a
     concurrency of one went from 5ms to 178ms and the numbers described the
     measurement. */
  let peak = 0
  let sampling = false
  const watching = setInterval(() => {
    if (sampling) return
    sampling = true
    warehouseConnections()
      .then((n) => { if (n !== null && n > peak) peak = n })
      .finally(() => { sampling = false })
  }, 500)

  const r = await drive('billing-summary', REQUESTS, concurrency)
  clearInterval(watching)
  r.connections = peak
  byConcurrency[concurrency] = r
  row(String(concurrency), ms(r.p50), ms(r.p95), ms(r.p99), ms(r.max),
    r.throughput.toFixed(1).padStart(8), String(r.failures.length).padEnd(7),
    r.connections > 0 ? `conns ${r.connections}` : '')

  /* A refusal is a finding, not noise. The first run of this said 80 failed
     at concurrency 1 and the cause was cronos's own rate limit — which is a
     limit that would have throttled a real team and been reported as "the
     reports are broken sometimes". */
  if (r.failures.length > 0) {
    const counts = {}
    for (const status of r.failures) counts[status] = (counts[status] ?? 0) + 1
    console.log('      refused: ' + Object.entries(counts)
      .map(([status, n]) => `${n}x HTTP ${status}`).join(', '))
  }
}

/* The number that matters is not the latency at 64 — it is whether throughput
   went up. A tool that serialises shows flat throughput and latency rising in
   proportion, which reads as "it got slower under load" and is really "it was
   never doing more than one thing". */
const scaling = byConcurrency[64].throughput / byConcurrency[1].throughput
console.log(`\n  throughput at 64 in flight is ${scaling.toFixed(1)}x the throughput at 1`)
console.log(scaling < 1.5
  ? '  → it is serialising somewhere'
  : '  → it scales with concurrency')

console.log(`\n=== bursting to ${CUSTOMERS} recipients ===\n`)

const burstStarted = performance.now()
const fired = await fetch(`${API}/v1/schedules/monthly-statements/run`, {
  method: 'POST', headers: { authorization: `Bearer ${ADMIN}` },
})
const burstWall = performance.now() - burstStarted

if (!fired.ok) {
  console.log(`  the burst did not run: ${fired.status} ${await fired.text()}`)
} else {
  const runs = await fetch(`${API}/v1/runs?limit=1`, {
    headers: { authorization: `Bearer ${ADMIN}` },
  }).then((r) => r.json())

  const run = runs.runs?.[0]
  row('recipients', String(run?.recipients ?? '?'))
  row('delivered', String(run?.delivered ?? '?'))
  row('wall clock', `${(burstWall / 1000).toFixed(1)}s`)
  row('documents per second', ((run?.delivered ?? 0) / (burstWall / 1000)).toFixed(1))
  /* The one that decides whether this is a product or a demo: at five thousand
     recipients, this rate is the difference between finishing before anybody
     is awake and still going at nine. */
  const atFiveThousand = 5000 / ((run?.delivered ?? 1) / (burstWall / 1000))
  console.log(`\n  at this rate, 5,000 statements would take ${(atFiveThousand / 60).toFixed(1)} minutes`)

  /* A throughput number is worth nothing if the documents are not documents.
     Three hundred PDFs a second is either very good or a sign that nothing is
     being typeset, and the difference is four lines of checking. */
  const files = readdirSync(`${process.env.WORK}/deliveries`, { recursive: true })
    .map((f) => `${process.env.WORK}/deliveries/${f}`)
    .filter((f) => f.endsWith('.pdf'))
  if (files.length === 0) {
    console.log('  ⚠ no documents were written — the rate above measures nothing')
  } else {
    const sizes = files.map((f) => statSync(f).size)
    const smallest = Math.min(...sizes)
    const header = readFileSync(files[0]).subarray(0, 5).toString()
    row('documents written', String(files.length))
    row('smallest document', `${(smallest / 1024).toFixed(1)} KB`)
    row('and it is a PDF', header === '%PDF-' ? 'yes' : `no — starts ${JSON.stringify(header)}`)
  }
}

console.log('\n=== what the server counted ===\n')
const metrics = await fetch(`${API}/v1/metrics`).then((r) => r.text())
for (const line of metrics.split('\n')) {
  if (/^cronos_(runs|deliveries)_total|^cronos_requests_total.*reports/.test(line)) {
    console.log('  ' + line)
  }
}

/* Connections are the thing that runs out. A pool that grows through a burst
   and never comes back is one that will hit the database's own limit on the
   day two schedules overlap. */
try {
  const log = readFileSync(`${process.env.WORK}/server.log`, 'utf8')
  const slow = log.split('\n').filter((l) => /ms=[0-9]{4,}/.test(l))
  if (slow.length > 0) {
    console.log(`\n  ${slow.length} requests took over a second:`)
    slow.slice(0, 3).forEach((l) => console.log('    ' + l.slice(0, 160)))
  }
} catch {
  // The log is a nicety; its absence is not a result.
}
