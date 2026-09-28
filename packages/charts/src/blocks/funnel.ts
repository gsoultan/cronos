import type { ChartBlock } from '../types'
import { svg, n } from '../svg'
import { canvas, fit, label, measure, sized } from '../frame'
import { chartPanel, host, nothing } from '../chart'
import { withTips } from '../tip'
import { stepOf } from '../palette'

const PX = 12
const THICK = 28
const GAP = 22

/**
 * A funnel: each stage a centred bar as wide as its share of the first, the
 * stages joined by a pale band that narrows from one to the next with the
 * fall written in it — the fall is what a funnel is read for, and it belongs
 * between the two stages it describes. Each stage's name is on the left and
 * its value and share of the first on the right.
 *
 * The ordinal ramp, not eight identities. Swapping two stages changes what the
 * chart says, so the colour carries the order: a reader sees the sequence
 * rather than discovering there was one by reading top to bottom.
 */
export function funnelBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const stages = b.stages ?? []
  if (stages.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  let entered = false
  sized(at, (width) => {
    const m = measure(at)
    const height = stages.length * THICK + (stages.length - 1) * GAP + 4
    const tail = (i: number) => `${stages[i]?.formatted ?? ''} · ${Math.round((stages[i]?.share ?? 0) * 1000) / 10}%`
    const nameW = Math.min(Math.ceil(Math.max(...stages.map((s) => m(s.label, PX)))), width * 0.28)
    const valueW = Math.ceil(Math.max(...stages.map((_, i) => m(tail(i), PX))))
    const x0 = nameW + 16
    const span = Math.max(1, width - x0 - valueW - 16)
    const root = canvas(width, height)
    const box = (i: number) => {
      const w = Math.max((stages[i]?.share ?? 0) * span, span * 0.02)
      return { x: x0 + (span - w) / 2, y: i * (THICK + GAP) + 2, w }
    }

    stages.forEach((s, i) => {
      const { x, y, w } = box(i)
      const fill = `var(--cr-step-${stepOf(i)})`
      const next = stages[i + 1]
      if (next) {
        const to = box(i + 1)
        const neck = svg('path', {
          class: 'neck',
          d: `M${n(x)} ${n(y + THICK)}L${n(x + w)} ${n(y + THICK)}L${n(to.x + to.w)} ${n(to.y)}L${n(to.x)} ${n(to.y)}Z`,
        })
        neck.style.fill = fill
        root.append(neck)
        if (next.drop) root.append(label(x0 + span / 2, y + THICK + GAP / 2 + 4, next.drop, 'drop'))
      }
      const band = svg('rect', { class: 'band', part: 'stage', x: n(x), y: n(y), width: n(w), height: THICK, rx: 4 })
      band.style.fill = fill
      tips.bind(band, s.label, s.formatted)
      root.append(band,
        label(0, y + THICK / 2 + 4, fit(s.label, nameW, PX, m), 'name', 'start'),
        label(width, y + THICK / 2 + 4, tail(i), 'value', 'end'))
    })
    if (!entered) root.classList.add('enter')
    entered = true
    at.replaceChildren(root)
  })
  return panel
}
