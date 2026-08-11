/*
 * The budget is the product decision, so it is enforced rather than aspired to.
 *
 * An ISV weighs this against their own page, and "≲40 KB gzip" was the number
 * that made embedding acceptable at all. A bundle that drifts past it does not
 * announce itself — pages just get slower and the reason is three commits back.
 */
import { gzipSync } from 'node:zlib'
import { readFileSync, statSync } from 'node:fs'

const BUDGET_GZIP = 40 * 1024
const file = 'dist/cronos-embed.js'

const raw = readFileSync(file)
const gzip = gzipSync(raw, { level: 9 }).length
const pct = Math.round((gzip / BUDGET_GZIP) * 100)

const kb = (n) => `${(n / 1024).toFixed(1)} KB`
console.log(`  ${file}`)
console.log(`  raw       ${kb(statSync(file).size)}`)
console.log(`  gzip      ${kb(gzip)}  (${pct}% of ${kb(BUDGET_GZIP)} budget)`)

if (gzip > BUDGET_GZIP) {
  console.error(`\nFAIL over budget by ${kb(gzip - BUDGET_GZIP)}`)
  process.exit(1)
}
console.log('\nok  within budget')
