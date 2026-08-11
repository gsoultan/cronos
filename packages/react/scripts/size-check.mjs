/*
 * The wrapper's own budget. It bundles @cronos/embed, so this is what a React
 * host actually downloads: the whole reporting widget in one number.
 */
import { gzipSync } from 'node:zlib'
import { readFileSync, statSync } from 'node:fs'

const BUDGET_GZIP = 40 * 1024
const file = 'dist/cronos-react.js'
const gzip = gzipSync(readFileSync(file), { level: 9 }).length
const kb = (n) => `${(n / 1024).toFixed(1)} KB`

console.log(`  ${file}`)
console.log(`  raw       ${kb(statSync(file).size)}`)
console.log(`  gzip      ${kb(gzip)}  (${Math.round((gzip / BUDGET_GZIP) * 100)}% of ${kb(BUDGET_GZIP)} budget)`)
console.log(`  react     external — never bundled`)

if (gzip > BUDGET_GZIP) {
  console.error(`\nFAIL over budget by ${kb(gzip - BUDGET_GZIP)}`)
  process.exit(1)
}
console.log('\nok  within budget')
