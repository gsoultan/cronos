/*
 * Drives the built package inside a real client-side React application.
 *
 * Not jsdom and not a unit test: the claims worth checking are about React's
 * render loop meeting a custom element — that an inline object prop does not
 * refetch forever, that StrictMode's double-mount does not double-fetch, and
 * that unmounting mid-request does not resolve into a tree that is gone.
 * None of those reproduce without a browser and a real reconciler.
 */
import { createServer } from 'node:http'
import { readFileSync } from 'node:fs'
import { chromium } from 'playwright'

const payload = (overdue) => ({
  title: 'Monthly invoice statement',
  filters: [{ name: 'status', label: 'Status', type: 'enum' }],
  blocks: [{
    kind: 'stat',
    title: 'Total billed',
    value: overdue ? '€5.5M' : '€49.9M',
    coverage: { applied: ['status'] },
  }],
})

let requests = 0
let fail = false

const server = createServer((req, res) => {
  if (req.url === '/' || req.url === '') {
    res.writeHead(200, { 'content-type': 'text/html' })
    return res.end(readFileSync('harness/index.html'))
  }
  if (req.url === '/app.js') {
    res.writeHead(200, { 'content-type': 'text/javascript' })
    return res.end(readFileSync('harness/app.js'))
  }
  if (req.url.startsWith('/v1/embed/reports/')) {
    requests++
    let body = ''
    req.on('data', (c) => (body += c))
    return req.on('end', () => {
      if (fail) {
        res.writeHead(403, { 'content-type': 'application/json' })
        return res.end(JSON.stringify({ error: 'This report link has expired.' }))
      }
      res.writeHead(200, { 'content-type': 'application/json' })
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

/* Uncaught exceptions and React's own warnings. Failed resource loads are
   excluded because this check deliberately causes one — a 403 is a case under
   test, not a defect, and a favicon nobody serves is neither. */
const errors = []
page.on('pageerror', (e) => errors.push(String(e)))
page.on('console', (m) => {
  if (m.type() !== 'error') return
  if (m.text().startsWith('Failed to load resource')) return
  errors.push(m.text())
})

const value = () => page.locator('cronos-report').locator('.stat').innerText()
const settle = () => page.waitForTimeout(600)

await page.goto(base, { waitUntil: 'domcontentloaded' })
await page.locator('cronos-report').locator('.stat').waitFor()

ok('renders inside a React application', (await value()) === '€49.9M')
ok('onLoad reaches a React callback',
  (await page.locator('[data-testid=events]').innerText()).includes('load'))

/* StrictMode mounts, unmounts and remounts every component in development.
   A component that fetches on mount without cleaning up fetches twice, and an
   ISV developing against us would see doubled API usage and blame the widget. */
ok(`StrictMode's double mount does not double-fetch (${requests} request)`, requests === 1)

/* The one that matters. `filters={{…}}` is a new object on every render, so a
   dependency on identity would reload forever — and each reload sets state,
   which renders, which makes another object. It is an infinite loop that looks
   like a working demo until someone opens the network tab. */
const beforeIdle = requests
await page.click('[data-testid=rerender]')
await page.click('[data-testid=rerender]')
await settle()
ok(`re-rendering the parent does not refetch (${requests - beforeIdle} requests)`,
  requests === beforeIdle)

/* But a real change must still take effect. */
await page.click('[data-testid=filter]')
await page.waitForFunction(() => document.querySelector('cronos-report')
  .shadowRoot.querySelector('.stat')?.textContent === '€5.5M')
ok('changing the filter prop refetches', (await value()) === '€5.5M')

const afterFilter = requests
await page.click('[data-testid=rerender]')
await settle()
ok('and it settles again afterwards', requests === afterFilter)

/* Failures arrive as a callback, not as an exception React has to catch. */
fail = true
await page.click('[data-testid=clear]')
await page.waitForFunction(() =>
  document.querySelector('[data-testid=events]')?.textContent?.includes('error'))
ok('onError reaches a React callback', true)
fail = false

/* Unmounting mid-flight must not resolve into a detached tree. React logs a
   console error for state set on an unmounted component, so an empty console
   is the assertion. */
await page.click('[data-testid=unmount]')
await settle()
ok('unmounting removes the element', await page.locator('cronos-report').count() === 0)
/* React logs a console error when state is set on an unmounted component, so
   a silent console is what proves the abort actually happened. */
ok(`React logged nothing (${errors.length})`, errors.length === 0)
if (errors.length) console.log(errors.slice(0, 3).map((e) => `       ${e}`).join('\n'))

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
server.close()
process.exit(fails ? 1 : 0)
