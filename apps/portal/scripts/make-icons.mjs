/*
 * Renders the cronos mark to PNG icons.
 *
 * Run by hand when the mark changes, not at build time: the output is
 * committed, so CI never needs a browser to produce a favicon. A manifest with
 * no icons is not installable at all, which is what shipped before this.
 */
import { chromium } from 'playwright'
import { writeFileSync } from 'node:fs'

const MARK = (stroke) => `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none"
  stroke="${stroke}" stroke-width="3.5" stroke-linecap="round">
  <path d="M22.46 7.73 A10.5 10.5 0 1 0 22.46 24.27"/>
  <circle cx="16" cy="16" r="2.2" fill="${stroke}" stroke="none"/></svg>`

// A maskable icon must survive a circular crop, so the mark sits inside a
// safe zone of 80% with the brand colour filling the bleed.
const page = (size, { maskable }) => `<!doctype html><html><body style="margin:0">
  <div style="width:${size}px;height:${size}px;display:grid;place-items:center;
    background:${maskable ? '#2a78d6' : 'transparent'}">
    <div style="width:${Math.round(size * (maskable ? 0.55 : 0.78))}px">
      ${MARK(maskable ? '#ffffff' : '#2a78d6')}
    </div></div></body></html>`

const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
for (const [size, opts, name] of [
  [192, {}, 'icon-192.png'],
  [512, {}, 'icon-512.png'],
  [512, { maskable: true }, 'icon-maskable-512.png'],
  [180, { maskable: true }, 'apple-touch-icon.png'],
]) {
  const p = await browser.newPage({ viewport: { width: size, height: size },
    deviceScaleFactor: 1 })
  await p.setContent(page(size, opts))
  writeFileSync(`public/${name}`, await p.screenshot({ omitBackground: !opts.maskable }))
  await p.close()
  console.log(`  ${name}  ${size}×${size}${opts.maskable ? ' maskable' : ''}`)
}
await browser.close()
