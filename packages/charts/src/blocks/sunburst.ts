import type { Bar, ChartBlock, Group } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'
import { arc, label, sizeToFit } from '../frame'
import { chartPanel, hue, nothing } from '../chart'
import { legend } from '../legend'
import { withTips } from '../tip'

const SIZE = 240
const MID = SIZE / 2
const LABEL_PX = 10

/**
 * A sunburst: parts within parts, as rings. Each series is a segment of the
 * inner ring as large as its share of the whole, and its categories sit around
 * it in its colour, softened — the treemap's nesting, where the reader wants
 * the shares' angles rather than their areas.
 *
 * Largest first, both rings, from twelve o'clock; a hair of surface between
 * every two segments, because two fills that touch read as one; and the whole
 * in the middle, which is the number a share chart is read against.
 */
export function sunburstBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title, false)
  const parents = ranked(b.groups ?? [])
  const grand = parents.reduce((sum, p) => sum + p.total, 0)
  if (grand <= 0) return nothing(panel, 'Nothing to divide up.')

  const tips = withTips(panel)
  const dial = svg('svg', { viewBox: `0 0 ${SIZE} ${SIZE}`, class: 'pie sunburst enter', part: 'chart', 'aria-hidden': 'true' })
  const pad = 0.01
  let from = 0
  for (const p of parents) {
    const span = (p.total / grand) * Math.PI * 2
    segment(dial, p.g.slot, 50, 86, from, span, pad, 'slice', tips,
      p.g.label, `${formatShare(p.total, grand)} of the whole`)
    let at = from
    for (const child of p.children) {
      const part = (child.value / grand) * Math.PI * 2
      segment(dial, p.g.slot, 89, 116, at, part, pad, 'slice child', tips,
        `${p.g.label} · ${child.label}`, `${child.formatted} · ${formatShare(child.value, p.total)} of ${p.g.label}`)
      at += part
    }
    from += span
  }
  // The server's whole, formatted with the parts: a sum made here would be
  // the one number on the chart this viewer formatted.
  const whole = b.totals?.[0]
  if (whole) {
    const figure = label(MID, MID + 3, whole.formatted, 'centre')
    figure.style.fontSize = `${n(sizeToFit(whole.formatted, 80, 20))}px`
    dial.append(figure, label(MID, MID + 20, whole.label, 'centre-label'))
  }
  panel.append(el('div', { class: 'pie-wrap' }, dial))
  const key = legend(parents.map((p) => ({ label: p.g.label, slot: p.g.slot })))
  if (key) panel.append(key)
  return panel
}

function segment(into: SVGSVGElement, slot: number, r0: number, r1: number, from: number, span: number,
  pad: number, cls: string, tips: ReturnType<typeof withTips>, name: string, said: string) {
  if (span <= 0) return
  const path = svg('path', { class: cls, part: 'slice',
    d: arc(MID, MID, r0, r1, from + pad / 2, from + Math.max(span - pad / 2, pad / 2 + 1e-4)) })
  path.style.fill = hue(slot)
  tips.bind(path, name, said)
  into.append(path)
  // Named where the segment has the room along its middle for the name —
  // its own name, the part after the series it sits in.
  const own = name.includes(' · ') ? name.slice(name.lastIndexOf(' · ') + 3) : name
  const r = (r0 + r1) / 2
  if (span * r >= own.length * LABEL_PX * 0.62 + 6) {
    const mid = from + span / 2 - Math.PI / 2
    into.append(label(MID + Math.cos(mid) * r, MID + Math.sin(mid) * r + 3.5, own,
      cls.includes('child') ? 'sun-label outer' : 'sun-label'))
  }
}

interface Parent { g: Group; total: number; children: Bar[] }

/** The series largest first, each with its categories largest first. Nothing
 *  or less is not a part. */
function ranked(groups: Group[]): Parent[] {
  return groups
    .map((g) => {
      const children = g.bars.filter((x) => x.value > 0).sort((a, c) => c.value - a.value)
      return { g, total: children.reduce((sum, x) => sum + x.value, 0), children }
    })
    .filter((p) => p.total > 0)
    .sort((a, c) => c.total - a.total)
}

function formatShare(part: number, whole: number): string {
  return `${Math.round((part / whole) * 1000) / 10}%`
}
