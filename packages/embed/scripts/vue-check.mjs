/*
 * The framework-agnostic claim, checked in Vue.
 *
 * One thing decides whether that claim is true: does Vue set `filters` as a
 * property or stringify it into an attribute? Vue's runtime sets a property
 * when the key exists on the element, which ours does — but "should work" and
 * "does work" differ by exactly one test, and this is a claim we make in a
 * README to people choosing a vendor.
 */
import { createServer } from 'node:http'
import { readFileSync } from 'node:fs'
import { chromium } from 'playwright'

const HTML = `<!doctype html><meta charset="utf-8">
<div id="root"></div><script type="module" src="/vue-app.js"></script>`

let lastBody = null
const server = createServer((req, res) => {
  if (req.url === '/vue-app.js') {
    res.writeHead(200, { 'content-type': 'text/javascript' })
    return res.end(readFileSync('harness/vue-app.js'))
  }
  if (req.url.startsWith('/v1/embed/reports/')) {
    let body = ''
    req.on('data', (c) => (body += c))
    return req.on('end', () => {
      lastBody = body
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end(JSON.stringify({
        title: 'Report',
        blocks: [{
          kind: 'stat', title: 'Total billed',
          value: body.includes('overdue') ? '€5.5M' : '€49.9M',
        }],
      }))
    })
  }
  res.writeHead(200, { 'content-type': 'text/html' }).end(HTML)
})

await new Promise((r) => server.listen(0, r))
const base = `http://localhost:${server.address().port}`

const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const page = await browser.newPage()
let fails = 0
const ok = (name, cond) => { console.log(`  ${cond ? 'ok  ' : 'FAIL'} ${name}`); if (!cond) fails++ }

await page.goto(base, { waitUntil: 'domcontentloaded' })
const stat = page.locator('cronos-report').locator('.stat')
await stat.waitFor()

ok('renders inside a Vue application', (await stat.innerText()) === '€49.9M')
ok('no wrapper needed — the element is the API',
  await page.locator('cronos-report').count() === 1)

await page.click('[data-testid=filter]')
await page.waitForFunction(() => document.querySelector('cronos-report')
  .shadowRoot.querySelector('.stat')?.textContent === '€5.5M')
ok('Vue sets filters as a property, not an attribute', (await stat.innerText()) === '€5.5M')
ok('and it arrives structured, not stringified',
  lastBody.includes('"op":"eq"') && !lastBody.includes('[object Object]'))

console.log(fails ? `\n${fails} failed` : '\nall passed')
await browser.close()
server.close()
process.exit(fails ? 1 : 0)
