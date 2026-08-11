#!/usr/bin/env node
/* Drives the portal in headless Chrome and captures the screens, so a change
   can be looked at rather than assumed. */
import { chromium } from 'playwright'
import { mkdirSync } from 'node:fs'

const BASE = process.env.BASE ?? 'http://localhost:5273'
const OUT = 'shots'
mkdirSync(OUT, { recursive: true })

const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const page = await (await browser.newContext({
  viewport: { width: 1440, height: 1000 },
  deviceScaleFactor: 2,
})).newPage()

const errors = []
page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
page.on('pageerror', (e) => errors.push(String(e)))

async function shot(name) {
  await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true })
  console.log(`  captured ${name}`)
}

await page.goto(BASE, { waitUntil: 'networkidle' })
await page.waitForSelector('text=Reports')
await shot('01-reports-list')

// Workspace switcher: orgs, projects, and the role each grant comes from.
await page.click('[data-testid=workspace-trigger]')
await page.waitForSelector('[data-testid=workspace-menu]')
await shot('05-workspace-switcher')

// A viewer-only project must not offer "New report".
await page.click('[data-testid=workspace-item]:has-text("Operations")')
await page.waitForTimeout(250)
await shot('06-project-viewer')

// Switching org: Northwind grants admin, so its projects come "via org".
await page.click('[data-testid=workspace-trigger]')
await page.click('[data-testid=workspace-item]:has-text("Northwind Trading")')
await page.waitForTimeout(250)
await shot('07-empty-project')

await page.click('[data-testid=workspace-trigger]')
await page.click('[data-testid=workspace-item]:has-text("Acme Logistics")')
await page.waitForTimeout(250)

await page.click('text=Monthly invoice statement')
await page.waitForSelector('text=Total billed')
await page.waitForTimeout(400)   // let the ResizeObserver settle chart widths
await shot('02-report')

await page.click('[data-testid=filter-toggle]')
await page.waitForSelector('text=Match all')
await page.click('text=+ Add condition')
await page.waitForTimeout(250)
await shot('03-filter-builder')

// Dark mode is a selected palette, not an inversion — it needs looking at too.
await page.click('button[aria-label*="dark theme"]')
await page.waitForTimeout(300)
await shot('04-report-dark')

// New screens: forms first, then the layout builder.
await page.goto(`${BASE}/data`, { waitUntil: 'networkidle' })
await page.waitForSelector('text=Connect a source')
await shot('08-data-sources')
await page.click('button:has-text("Connect a source")')
await page.waitForSelector('text=What are you connecting?')
await shot('09-source-wizard-kind')
await page.click('button:has-text("REST API")')
await page.click('button:has-text("Continue")')
await page.waitForTimeout(300)
await shot('10-source-wizard-connect')

await page.goto(`${BASE}/schedules`, { waitUntil: 'networkidle' })
await page.click('button:has-text("New schedule")')
await page.waitForSelector('text=When?')
await page.click('button:has-text("First of the month")')
await page.click('input[type="checkbox"]')
await page.waitForTimeout(300)
await shot('11-schedule-form')

await page.goto(`${BASE}/reports/new`, { waitUntil: 'networkidle' })
await page.fill('input[aria-label="Report name"]', 'Monthly invoice statement')
await page.click('input[placeholder="Choose a dataset"]')
await page.locator('[role="option"]:visible').first().click()
await page.waitForSelector('[data-testid=block-palette]')
for (const b of ['Add Number', 'Add Number', 'Add Bar chart', 'Add Line chart', 'Add Table']) {
  await page.click(`button[aria-label="${b}"]`)
  await page.waitForTimeout(150)
}
await page.click('[data-testid=layout-canvas] > div >> nth=2')   // select the bar chart
await page.waitForTimeout(600)
await shot('12-report-editor')

await page.goto(`${BASE}/settings`, { waitUntil: 'networkidle' })
await page.waitForSelector('text=Settings')
await shot('13-settings')

console.log(errors.length ? `\nconsole errors:\n${errors.join('\n')}` : '\nno console errors')
await browser.close()
