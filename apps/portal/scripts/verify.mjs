/*
 * Runs every browser suite against a running dev server and summarises.
 *
 * Sequential on purpose. Ten headless Chromes at once thrash a laptop badly
 * enough that timing-sensitive assertions start failing for reasons that have
 * nothing to do with the code, and a flaky gate gets ignored.
 *
 * Keeps going after a failure rather than stopping at the first one: knowing
 * that three suites broke together is what tells you it was the shell, not the
 * share panel.
 */
import { spawnSync } from 'node:child_process'
import { existsSync, readdirSync } from 'node:fs'

const B = process.env.BASE ?? 'http://localhost:5173'

/* platform-check reads the *built* manifest, so it needs dist/ to exist. Say so
   rather than letting it die in JSON.parse, which reads like a broken suite. */
if (!existsSync('dist/manifest.webmanifest')) {
  console.error('dist/manifest.webmanifest is missing — run `bun run build` first.')
  process.exit(1)
}

/* The sample-mode suites only. Anything named live-* drives the portal against
   a real cronosd on its own ports and is started by scripts/live-portal.sh —
   running it here would fail for want of a server rather than for want of a
   working interface. */
const suites = readdirSync('scripts')
  .filter((f) => f.endsWith('-check.mjs') && !f.startsWith('live-'))
  .sort()

const failed = []
for (const file of suites) {
  const name = file.replace('-check.mjs', '')
  process.stdout.write(`${name.padEnd(12)}`)
  const r = spawnSync('node', [`scripts/${file}`], {
    env: { ...process.env, BASE: B }, encoding: 'utf8',
  })
  const out = `${r.stdout ?? ''}${r.stderr ?? ''}`
  if (r.status === 0) {
    console.log(`ok   ${out.match(/ ok {2}/g)?.length ?? 0} assertions`)
  } else {
    failed.push(name)
    const lines = out.split('\n').filter((l) => l.includes('FAIL') || l.includes('Error'))
    console.log(`FAIL`)
    for (const l of lines.slice(0, 5)) console.log(`             ${l.trim()}`)
  }
}

console.log(failed.length ? `\n${failed.length} suite(s) failed: ${failed.join(', ')}` : '\nall suites passed')
process.exit(failed.length ? 1 : 0)
