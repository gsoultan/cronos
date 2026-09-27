import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { css, documentCss, LAYER } from '@cronos/charts'

/*
 * The chart stylesheet stays inside the charts.
 *
 * It is written for a shadow root, where the boundary does the scoping, so its
 * rules are bare class selectors — `.grid`, `.panel`, `.bars`, and a `*` that
 * sets box-sizing. The portal has no boundary: it adopts the same string into
 * the document, and an adopted stylesheet is unlayered, which beats every
 * layered utility on the page.
 *
 * `.grid` was the one that mattered. The application shell is
 * `<div class="grid grid-cols-[248px_minmax(0,1fr)]">`, and from the first
 * chart a session drew until it ended, that div was given
 * `repeat(auto-fit, minmax(220px, 1fr))` instead: the navigation column became
 * half the window and every report was squeezed into the other half. The
 * report's own block grid went the same way, and the sheet's literal
 * `--cr-surface: #fff` beat the portal's themed one, so charts stayed white on
 * a dark page.
 *
 * Nothing caught it because it is invisible until a chart renders — the
 * reports list, every settings screen and every test that does not draw one
 * are all laid out correctly.
 */
describe('the stylesheet the portal adopts', () => {
  test('carries the bare selectors that leaked', () => {
    // The shadow-root form is unchanged, and it is what makes scoping the
    // document form necessary rather than tidy.
    expect(css(':host')).toContain('.grid {')
    expect(css(':host')).toContain('* { box-sizing: border-box }')
  })

  test('puts every one of them inside the scope', () => {
    const out = documentCss('.cronos-chart')
    const scope = out.indexOf('@scope (.cronos-chart)')
    expect(scope).toBeGreaterThan(-1)
    // Not merely present: after the scope opens, so it cannot match the shell.
    expect(out.indexOf('.grid {')).toBeGreaterThan(scope)
    expect(out.indexOf('* { box-sizing: border-box }')).toBeGreaterThan(scope)
  })

  test('addresses the root as :scope, not as the bare class', () => {
    // `$SCOPE$` holds the custom properties. Left as `.cronos-chart` inside
    // `@scope (.cronos-chart)` it would still match, but as a descendant
    // selector — so it would stop matching the root itself.
    expect(documentCss('.cronos-chart')).toContain(':scope {')
  })

  test('and declares a layer either way', () => {
    expect(documentCss('.cronos-chart')).toContain(`@layer ${LAYER} {`)

    // The fallback, for a browser with no `@scope`: an at-rule it does not
    // know takes every rule inside it with it, so the charts would have no
    // styling at all rather than merely unscoped styling.
    const flat = documentCss('.cronos-chart', false)
    expect(flat).toContain(`@layer ${LAYER} {`)
    expect(flat).not.toContain('@scope')
    expect(flat).toContain('.cronos-chart {')
  })
})

/*
 * The layer is only worth declaring if the host orders it, and where it is
 * ordered is the whole of its behaviour. A layer nobody names is appended last
 * and wins — which is the bug — and a layer named too low loses to Tailwind's
 * preflight, which is `*{ margin: 0; padding: 0; border: 0 }`: chart panels
 * kept their background and their corner radius and lost their padding and
 * their border, a card with no edges.
 */
describe('the portal orders that layer', () => {
  const theme = readFileSync(new URL('../theme/index.css', import.meta.url), 'utf8')
  const statement = theme.match(/@layer ([^;]+);/)
  const order = (statement?.[1] ?? '').split(',').map((s) => s.trim())

  test('names it', () => {
    expect(order).toContain(LAYER)
  })

  test('above base, so preflight does not strip a panel to nothing', () => {
    expect(order.indexOf(LAYER)).toBeGreaterThan(order.indexOf('base'))
  })

  test('below utilities, so a utility class still wins', () => {
    expect(order.indexOf(LAYER)).toBeLessThan(order.indexOf('utilities'))
  })
})
