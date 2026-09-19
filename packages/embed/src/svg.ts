/**
 * SVG element construction.
 *
 * Separate from `el` in dom.ts because `createElement('circle')` produces an
 * HTMLUnknownElement that lays out as nothing: the failure is a chart that
 * renders blank with no error anywhere, which is the most expensive kind.
 * SVG needs createElementNS, and nothing else about the helper differs.
 *
 * `innerHTML` is not used here either — see dom.ts for why.
 */
const NS = 'http://www.w3.org/2000/svg'

export function svg<K extends keyof SVGElementTagNameMap>(
  tag: K,
  attrs?: Record<string, string | number>,
  ...children: (Node | string)[]
): SVGElementTagNameMap[K] {
  const node = document.createElementNS(NS, tag)
  for (const [k, v] of Object.entries(attrs ?? {})) node.setAttribute(k, String(v))
  for (const c of children) node.append(c)
  return node
}

/**
 * A number as SVG geometry: finite, and short.
 *
 * A NaN in a path attribute drops the whole element silently — one bad value
 * and the chart is empty with nothing in the console. Three decimals is finer
 * than a device pixel at any size a report is read at.
 */
export function n(v: number): string {
  return Number.isFinite(v) ? String(Math.round(v * 1000) / 1000) : '0'
}
