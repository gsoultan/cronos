/*
 * The wrapper's own budget.
 *
 * The element is not in it: a React host loads that from its cronos server at
 * run time (src/load.ts), and packages/embed holds that file to its own 40 KB.
 * This is what the host's bundle grows by, and it should stay glue.
 */
import { gzipSync } from 'node:zlib'
import { readFileSync, statSync } from 'node:fs'

const BUDGET_GZIP = 4 * 1024
const file = 'dist/cronos-react.js'
const source = readFileSync(file)
const gzip = gzipSync(source, { level: 9 }).length
const kb = (n) => `${(n / 1024).toFixed(1)} KB`

console.log(`  ${file}`)
console.log(`  raw       ${kb(statSync(file).size)}`)
console.log(`  gzip      ${kb(gzip)}  (${Math.round((gzip / BUDGET_GZIP) * 100)}% of ${kb(BUDGET_GZIP)} budget)`)
console.log(`  react     external — never bundled`)
console.log(`  element   loaded from the cronos server — never bundled`)

/* A runtime import of @cronos/embed would resolve to the host's npm copy and
   define the element before the server's could, which is the version drift
   loading it from the server exists to end. Types only. */
if (source.includes('@cronos/embed')) {
  console.error('\nFAIL the build imports @cronos/embed at run time — import its types only')
  process.exit(1)
}
if (gzip > BUDGET_GZIP) {
  console.error(`\nFAIL over budget by ${kb(gzip - BUDGET_GZIP)}`)
  process.exit(1)
}
console.log('\nok  within budget')
