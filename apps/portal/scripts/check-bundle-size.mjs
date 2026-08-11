#!/usr/bin/env node
/*
 * Enforces the frontend budgets from AGENTS.md against a real build.
 *
 *   Initial route  <= 500 KB raw, <= 150 KB gzip
 *   Any lazy chunk <= 500 KB raw
 *
 * "Initial route" is measured the way a browser sees it: the stylesheet, entry
 * script and every modulepreload in dist/index.html. A budget that is not
 * checked is a budget that is already blown.
 */
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist')
const KB = 1024
const BUDGET = { initialRaw: 500 * KB, initialGzip: 150 * KB, chunkRaw: 500 * KB }

const html = readFileSync(join(DIST, 'index.html'), 'utf8')

/* Everything the browser fetches before it can paint the first route. */
const eager = new Set()
for (const re of [
  /<script[^>]+src="\/([^"]+)"/g,
  /<link[^>]+rel="modulepreload"[^>]+href="\/([^"]+)"/g,
  /<link[^>]+rel="stylesheet"[^>]+href="\/([^"]+)"/g,
]) {
  for (const m of html.matchAll(re)) eager.add(m[1])
}

const sizeOf = (rel) => {
  const buf = readFileSync(join(DIST, rel))
  return { raw: buf.length, gzip: gzipSync(buf, { level: 9 }).length }
}

let initialRaw = 0
let initialGzip = 0
for (const rel of eager) {
  const { raw, gzip } = sizeOf(rel)
  initialRaw += raw
  initialGzip += gzip
}

const assets = readdirSync(join(DIST, 'assets'))
  .filter((f) => f.endsWith('.js') || f.endsWith('.css'))
  .map((f) => ({ file: `assets/${f}`, ...sizeOf(`assets/${f}`) }))

const fmt = (n) => `${(n / KB).toFixed(1)} KB`
const pad = (s, n) => String(s).padEnd(n)
let failed = false

const check = (label, actual, budget, format = fmt) => {
  const ok = actual <= budget
  if (!ok) failed = true
  console.log(`  ${ok ? 'ok  ' : 'FAIL'} ${pad(label, 34)} ${pad(format(actual), 12)} (budget ${format(budget)})`)
}

console.log('\nInitial route — what the browser loads before first paint')
for (const rel of [...eager].sort()) {
  const { raw } = sizeOf(rel)
  console.log(`       ${pad(rel, 40)} ${fmt(raw)}`)
}
check('initial route, raw', initialRaw, BUDGET.initialRaw)
check('initial route, gzip', initialGzip, BUDGET.initialGzip)

console.log('\nLazy chunks')
for (const a of assets.filter((a) => !eager.has(a.file)).sort((x, y) => y.raw - x.raw)) {
  check(a.file.replace('assets/', ''), a.raw, BUDGET.chunkRaw)
}

const total = statSync(DIST).isDirectory()
  ? assets.reduce((n, a) => n + a.raw, 0)
  : 0
console.log(`\n  ${assets.length} assets, ${fmt(total)} total\n`)

if (failed) {
  console.error('Bundle budget exceeded — see AGENTS.md § Bundle budgets.\n')
  process.exit(1)
}
