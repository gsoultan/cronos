/* Organisation logo: upload, both-surface preview, print check, and use. */
import { chromium } from 'playwright'

const B = process.env.BASE ?? 'http://localhost:5173'
const b = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const p = await (await b.newContext({ viewport: { width: 1600, height: 1100 } })).newPage()
const errs = []
p.on('pageerror', e => errs.push(String(e)))
p.on('console', m => m.type() === 'error' && errs.push(m.text()))
let f = 0
const ok = (n, c) => { console.log(`  ${c ? 'ok  ' : 'FAIL'} ${n}`); if (!c) f++ }

const svg = Buffer.from(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 40">' +
  '<rect width="120" height="40" fill="#2a78d6"/></svg>')
// 64px wide: fine on screen, far too small for a printed statement.
const tinyPng = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAEAAAABACAYAAACqaXHeAAAAJ0lEQVR42u3OMQEAAAgDoC1' +
  'apGa/CJ5gQNvMFAgEAoFAIBAIBIJHAy2rAAFuxRhqAAAAAElFTkSuQmCC', 'base64')

// Northwind: you are an admin there, so the controls are live.
await p.goto(B + '/settings', { waitUntil: 'domcontentloaded' })
await p.click('[data-testid=workspace-trigger]')
await p.click('[data-testid=workspace-item]:has-text("Northwind")')
await p.waitForTimeout(400)
await p.waitForSelector('[data-testid=branding]')
ok('Organization is the first settings tab', await p.locator('[data-testid=branding]').isVisible())
ok('it says where the logo will appear',
  await p.locator('text=paginated PDFs, embedded views').isVisible())

// A vector wordmark: no print warning, and both surfaces previewed.
await p.locator('input[aria-label="Wordmark"]').setInputFiles({
  name: 'logo.svg', mimeType: 'image/svg+xml', buffer: svg,
})
await p.waitForTimeout(400)
ok('previews on a light surface', await p.locator('[data-testid=preview-light]').first().isVisible())
ok('and on a dark surface', await p.locator('[data-testid=preview-dark]').first().isVisible())
ok('a vector says it prints at any size',
  await p.locator('text=vector, prints at any size').isVisible())
ok('no print warning for a vector',
  await p.locator('text=blurry on paper').count() === 0)

// A small raster mark: measured against print, not against the screen.
await p.locator('input[aria-label="Mark"]').setInputFiles({
  name: 'mark.png', mimeType: 'image/png', buffer: tinyPng,
})
await p.waitForTimeout(500)
ok('a small raster is flagged for print',
  await p.locator('text=blurry on paper').isVisible())
ok('and the required width is named',
  await p.locator('text=/needs about \\d+px/').isVisible())

// A rejected type says what to use instead.
await p.locator('input[aria-label="Wordmark"]').setInputFiles({
  name: 'notes.txt', mimeType: 'text/plain', buffer: Buffer.from('nope'),
})
await p.waitForTimeout(300)
ok('a wrong file type is refused with guidance',
  await p.locator('text=SVG is best').isVisible())

// The mark is actually used, not just stored.
ok('the uploaded mark appears in the workspace switcher',
  await p.locator('[data-testid=workspace-trigger] img').count() > 0)

// Non-admins cannot change it.
await p.click('[data-testid=workspace-trigger]')
await p.click('[data-testid=workspace-item]:has-text("Acme")')
await p.waitForTimeout(500)
ok('a member is told admins own the logo',
  await p.locator('text=Organization admins can change the logo').isVisible())
// Branding is per organisation. The first version mirrored it in component
// state, which did not reset on switch and showed Northwind's logo here.
// The panel holds draft edits, so it is keyed by organisation. Without that
// the fields keep the previous org's values after a switch.
ok('the name field shows this organisation, not the previous one',
  (await p.locator('input[aria-label="Organization name"]').inputValue()) === 'Acme Logistics')
ok('another organisation does not inherit the logo',
  await p.locator('[data-testid=drop-wide]').isVisible())
ok('and its drop zone is disabled for a member',
  await p.locator('[data-testid=drop-wide]').isDisabled())
ok('the rail falls back to initials without a mark',
  await p.locator('[data-testid=workspace-trigger] img').count() === 0)

console.log(errs.length ? '\nERRORS:\n' + errs.join('\n') : '\nno console errors')
console.log(f ? `${f} failed` : 'all passed')
await b.close()
process.exit(f ? 1 : 0)
