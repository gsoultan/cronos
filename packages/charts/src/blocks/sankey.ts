import type { ChartBlock, SankeyLink, SankeyNode } from '../types'
import { svg, n } from '../svg'
import { canvas, fit, label, measure, sized, type Measure } from '../frame'
import { chartPanel, host, hue, nothing } from '../chart'
import { withTips, type Tips } from '../tip'

const LABEL_PX = 12
const NODE = 10

/**
 * A sankey: where a measure goes — each category of x a bar down the left,
 * each of series a bar down the right, and a band from each source to each
 * target as thick as what it carries.
 *
 * Drawn from the layout the server worked out, in fractions of the box, so a
 * page and a screen draw the same picture; this only chooses how tall the box
 * is and how much of its width the names take. A band carries its source's
 * colour, softened, and pointing at a node lights every band through it.
 */
export function sankeyBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const s = b.sankey
  if (!s || s.nodes.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  let entered = false
  sized(at, (width) => {
    const root = draw(width, s.nodes, s.links, measure(at), tips)
    if (!entered) root.classList.add('enter')
    entered = true
    at.replaceChildren(root)
  })
  return panel
}

function draw(width: number, nodes: SankeyNode[], links: SankeyLink[], m: Measure, tips: Tips): SVGSVGElement {
  const said = (node: SankeyNode) => `${node.label} · ${node.formatted}`
  const room = (sideOf: number) => Math.min(width * 0.3,
    Math.max(0, ...nodes.filter((node) => node.side === sideOf).map((node) => m(said(node), LABEL_PX))) + 10)
  const x0 = room(0)
  const x1 = width - room(1) - NODE
  const perSide = Math.max(nodes.filter((node) => node.side === 0).length, nodes.filter((node) => node.side === 1).length)
  const height = Math.max(180, Math.min(perSide * 34, 520))
  const y = (v: number) => v * height

  const root = canvas(width, height)
  const bands = svg('g', { class: 'marks' })
  const bars = svg('g', { class: 'nodes' })
  const words = svg('g', { class: 'labels' })
  root.append(bands, bars, words)
  const drawn = links.map((l) => {
    const from = nodes[l.from]
    const to = nodes[l.to]
    const path = svg('path', { class: 'flow-band', part: 'flow', d: band(x0 + NODE, x1, y(l.y0), y(l.y1), y(l.h)) })
    path.style.fill = hue(from?.slot ?? 0)
    tips.bind(path, `${from?.label ?? ''} → ${to?.label ?? ''}`, l.formatted)
    bands.append(path)
    return path
  })
  nodes.forEach((node, i) => {
    const left = node.side === 0
    const rect = svg('rect', { class: 'flow-node', part: 'node', x: n(left ? x0 : x1), y: n(y(node.y)),
      width: NODE, height: n(Math.max(y(node.h), 1)), rx: 2 })
    if (left) rect.style.fill = hue(node.slot)
    tips.bind(rect, node.label, node.formatted)
    // Pointing at a node lights the bands through it and lets the rest fall
    // back, so one category's flows can be followed across.
    const through = drawn.filter((_, k) => links[k]?.from === i || links[k]?.to === i)
    rect.addEventListener('pointerenter', () => { root.classList.add('tracing'); for (const p of through) p.classList.add('on') })
    rect.addEventListener('pointerleave', () => { root.classList.remove('tracing'); for (const p of through) p.classList.remove('on') })
    bars.append(rect)
    const mid = y(node.y + node.h / 2) + 4
    words.append(left
      ? label(x0 - 6, mid, fit(said(node), x0 - 8, LABEL_PX, m), 'name', 'end')
      : label(x1 + NODE + 6, mid, fit(said(node), width - x1 - NODE - 8, LABEL_PX, m), 'name', 'start'))
  })
  return root
}

/** A band from x0 to x1: its top edge a curve from where it leaves its source
 *  to where it meets its target, its bottom edge back. */
function band(x0: number, x1: number, y0: number, y1: number, h: number): string {
  const xm = (x0 + x1) / 2
  return `M${n(x0)} ${n(y0)}C${n(xm)} ${n(y0)} ${n(xm)} ${n(y1)} ${n(x1)} ${n(y1)}` +
    `L${n(x1)} ${n(y1 + h)}C${n(xm)} ${n(y1 + h)} ${n(xm)} ${n(y0 + h)} ${n(x0)} ${n(y0 + h)}Z`
}
