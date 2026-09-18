/* Connecting an object store, which until now the form could not fully do.

   A private bucket needs a key and there was nowhere to put one: the form
   asked for a location and nothing else, so the only way to define a readable
   lake was to write the YAML by hand — which is exactly where somebody pastes
   the key itself instead of a ${secret:…} reference to it. */
import { chromium } from 'playwright'
const B = process.env.BASE ?? 'http://localhost:5173'
const b = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const p = await (await b.newContext({ viewport: { width: 1600, height: 1100 } })).newPage()
const errs = []
p.on('pageerror', e => errs.push(String(e)))
p.on('console', m => m.type() === 'error' && errs.push(m.text()))
let f = 0
const ok = (n, c) => { console.log(`  ${c ? 'ok  ' : 'FAIL'} ${n}`); if (!c) f++ }

await p.goto(B + '/data', { waitUntil: 'domcontentloaded' })
await p.click('button:has-text("Connect a source")')
await p.waitForTimeout(300)

// The kind decides which fields the form asks for, so pick the one this is about.
await p.click('button:has-text("Files in a bucket")')
await p.waitForTimeout(200)
await p.click('button:has-text("Continue")')
await p.waitForTimeout(500)

const uri = p.locator('[data-testid=source-uri]')
await uri.waitFor({ timeout: 10000 })
ok('a lake asks where it is', await uri.isVisible())
ok('and for the key that opens it', await p.locator('[data-testid=source-credentials]').isVisible())
ok('and the region it answers in', await p.locator('[data-testid=source-region]').isVisible())
ok('and the address, for a store that is not the cloud\'s own',
  await p.locator('[data-testid=source-endpoint]').isVisible())

/* The placeholder is the whole guidance. Somebody who types a key here rather
   than a reference to one has put a credential in a file that gets committed,
   and the field is the last place to say so. */
ok('the credential field asks for a reference rather than a key',
  (await p.locator('[data-testid=source-credentials]').getAttribute('placeholder') ?? '')
    .includes('${secret:'))

/* Optional, all three. A public bucket is a real thing and the form must not
   insist on a key to describe one. */
await uri.fill('s3://open-data/events/')
await p.locator('[data-testid=source-uri]').blur()
await p.waitForTimeout(250)
ok('a public bucket needs no key to continue',
  await p.locator('button:has-text("Continue")').first().isEnabled())

/* An endpoint is a URL and a typo in one sends every read somewhere that is
   not the store — which comes back as an empty lake rather than as an error. */
await p.locator('[data-testid=source-endpoint]').fill('minio.internal:9000')
await p.locator('[data-testid=source-endpoint]').blur()
await p.waitForTimeout(300)
ok('an endpoint without a scheme is refused',
  (await p.locator('body').innerText()).includes('http://'))

/* And accepted once it has one, so the rule is a rule and not a field that
   never passes. */
await p.locator('[data-testid=source-endpoint]').fill('http://minio.internal:9000')
await p.locator('[data-testid=source-endpoint]').blur()
await p.waitForTimeout(300)
ok('and accepted with one',
  !(await p.locator('body').innerText()).includes('Use an http://'))

console.log(errs.length ? '\nERRORS:\n' + errs.join('\n') : '\nno console errors')
console.log(f ? `${f} failed` : 'all passed')
await b.close()
process.exit(f ? 1 : 0)
