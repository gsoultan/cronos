import { useEffect, useRef } from 'react'
import { documentCss, drawBlock } from '@cronos/charts'
import type { Block } from '@cronos/charts'

/**
 * The chart stylesheet, adopted into this document once.
 *
 * The embed adopts the same string into its shadow root; here it is scoped to
 * `.cronos-chart` instead of `:host`, and wrapped, which is the one thing that
 * differs between a shadow root and a page. One stylesheet, so a report cannot
 * be drawn one way here and another in a customer's page.
 *
 * The wrapping is not decoration. Without it this one call broke the
 * application: the sheet's `.grid` rule reached the shell's own
 * `<div class="grid ...">` and turned the 248px navigation column into half the
 * window, for every page, from the first chart a session drew until it ended.
 * documentCss explains the rest.
 *
 * Adopted on first use rather than imported at the top of the application: no
 * first screen has a chart on it, and this follows the same rule as
 * mantine-deferred.css.
 */
let adopted = false
function adoptOnce() {
  if (adopted || typeof CSSStyleSheet === 'undefined') return
  adopted = true
  const sheet = new CSSStyleSheet()
  /* A browser that has not implemented `@scope` drops the at-rule and every
     rule inside it, which would leave the charts unstyled rather than merely
     unscoped. CSSScopeRule is the global that arrives with the feature, and
     the layer — which fixes the theming — is kept either way. */
  sheet.replaceSync(documentCss('.cronos-chart', 'CSSScopeRule' in globalThis))
  document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet]
}

/**
 * A block from the server, drawn by the same code the embed draws it with.
 *
 * This report view used to draw one chart type. `block.chart !== 'bar'` was the
 * whole of it, and everything else — a line, a map, a funnel, a gauge — came
 * back as "line charts need a newer portal" for a report the server had
 * rendered perfectly. There were three renderers of this payload and only two
 * of them got the attention.
 *
 * React does not own these nodes. The renderers are plain DOM functions with no
 * framework in them, which is what lets the embed ship them inside a 40 KB
 * budget, and wrapping each one in a component per chart type would be a second
 * implementation to keep in step with the first — the thing this replaces.
 */
export function ServerChart({ block }: { block: Block }) {
  const host = useRef<HTMLDivElement>(null)

  useEffect(() => {
    adoptOnce()
    const at = host.current
    if (!at) return
    at.replaceChildren(drawBlock(block))
    // Replaced rather than appended on every render, and cleared on unmount:
    // a filter changes faster than anybody looks, and two charts stacked in
    // one cell is what appending gives.
    return () => at.replaceChildren()
  }, [block])

  /* `cronos-chart` is where the palette lives — see theme/charts.css. The
     stylesheet is the embed's own, scoped to this class instead of `:host`,
     so the two cannot drift into drawing the same report differently. */
  /* h-full, and the panel with it: two charts sharing a row are two cards, and
     cards in a row that stop at different heights read as one of them having
     failed to load. The panel is the chart package's element, so the height
     has to be asked for from out here. */
  return <div ref={host} data-testid="chart" className="cronos-chart h-full [&>.panel]:h-full" />
}
