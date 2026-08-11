/* User management: listing, role changes, project grants, and the rules. */
import { chromium } from 'playwright'
const B = process.env.BASE ?? 'http://localhost:5273'
const b = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox'] })
const p = await (await b.newContext({ viewport: { width: 1600, height: 1050 } })).newPage()
const errs = []
p.on('pageerror', e => errs.push(String(e)))
p.on('console', m => m.type() === 'error' && errs.push(m.text()))
let f = 0
const ok = (n, c) => { console.log(`  ${c ? 'ok  ' : 'FAIL'} ${n}`); if (!c) f++ }

await p.goto(B + '/settings', { waitUntil: 'networkidle' })
await p.waitForSelector('[data-testid=people-list]')
const rows = p.locator('[data-testid=people-list] li')

// Acme: you are a member, so the list is readable but not editable.
ok('a member sees the list but no role controls',
  await rows.count() >= 5 && await p.locator('input[aria-label^="Role for"]').count() === 0)
ok('a member gets no Invite button',
  await p.locator('button:has-text("Invite people")').count() === 0)

// Northwind: you are an admin, so management is live.
await p.click('[data-testid=workspace-trigger]')
await p.click('[data-testid=workspace-item]:has-text("Northwind")')
await p.waitForTimeout(400)
await p.waitForSelector('[data-testid=people-list]')
ok(`the org member list actually lists people (${await rows.count()})`, await rows.count() >= 4)
ok('an admin gets the Invite button', await p.locator('button:has-text("Invite people")').isVisible())
ok('your own row is marked', await p.locator('[data-testid=people-list] >> text=you').first().isVisible())
ok('an org admin shows as reaching all projects',
  await p.locator('[data-testid=people-list] >> text=All projects').first().isVisible())
ok('someone with no grants says so, not blank',
  await p.locator('[data-testid=people-list] >> text=No projects yet').first().isVisible())

// The last owner cannot be demoted or removed, and the reason is on the control.
const ownerRow = rows.filter({ hasText: 'Ravi Menon' }).first()
ok('the last owner’s role select is locked',
  await ownerRow.locator('input[aria-label*="Role for"]').isDisabled())
ok('the reason is stated on the control',
  (await ownerRow.locator('[title]').first().getAttribute('title') ?? '').length > 0)
ok('the last owner cannot be removed',
  await ownerRow.locator('button:has-text("Remove")').isDisabled())

// A member's role can be changed in place.
const priya = rows.filter({ hasText: 'Lena Fischer' }).first()
await priya.locator('input[aria-label*="Role for"]').click()
await p.locator('[role="option"]:visible', { hasText: 'Admin' }).first().click()
await p.waitForTimeout(400)
ok('changing a role takes effect immediately',
  await priya.locator('text=All projects').isVisible())

// Removal confirms in place and says what is lost.
const tomas = rows.filter({ hasText: 'Owen Pratt' }).first()
await tomas.locator('button:has-text("Remove")').click()
await p.waitForTimeout(200)
ok('removal confirms in place', await p.locator('text=They lose access to').isVisible())
await p.locator('button:has-text("Remove")').last().click()
await p.waitForTimeout(300)
ok('removal works', await rows.filter({ hasText: 'Owen Pratt' }).count() === 0)

// Pending invitations are separate from members.
ok('pending invitations are listed separately',
  await p.locator('h2:has-text("Invited")').isVisible())

// Expanding a person shows per-project grants.
await p.locator('[data-testid=people-list] button[aria-expanded]').first().click()
await p.waitForTimeout(300)
ok('a person expands to their project grants',
  await p.locator('[data-testid=people-list] >> text=/reach every project|Customer analytics/').first().isVisible())

ok('a last-seen time is never in the future',
  await p.locator('[data-testid=people-list] >> text=/^in /').count() === 0)

await p.click('[role=tab]:has-text("Projects")')
await p.waitForTimeout(300)
ok('the projects tab counts people per project',
  await p.locator('text=/\\d+ (person|people)/').first().isVisible())

console.log(errs.length ? '\nERRORS:\n' + errs.join('\n') : '\nno console errors')
console.log(f ? `${f} failed` : 'all passed')
await b.close()
process.exit(f ? 1 : 0)
