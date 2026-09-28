import type { ChartBlock } from '../types'
import { svg } from '../svg'
import { cartesian } from '../axes'
import { bar, label, measure, sized } from '../frame'
import { chartPanel, host, hue, nothing, PLOT_HEIGHT } from '../chart'
import { withTips } from '../tip'

const VALUE_PX = 11

/**
 * A histogram: a number's range cut into bins on round numbers, a column per
 * bin standing on nothing and touching its neighbours — the bins are one
 * range, not a row of categories — with each bin's count over it where its
 * column has room. The scale along the bottom is the number's own, ticked at
 * the bins' edges, so a column reads as 500 to 1,000 rather than as a name.
 */
export function histogramBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const bins = b.bins ?? []
  const x = b.xAxis
  const y = b.yAxis
  if (!x || !y || bins.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  let entered = false
  sized(at, (width) => {
    const m = measure(at)
    const c = cartesian({ width, height: PLOT_HEIGHT, m, y, x, headroom: 18 })
    const words = svg('g', { class: 'labels' })
    const zero = c.y(Math.max(0, y.min))
    for (const bin of bins) {
      // An empty bin is a gap in the range, which is what it is.
      if (!bin.value) continue
      const left = c.x(bin.from)
      const right = c.x(bin.to)
      const top = c.y(bin.value)
      const path = svg('path', { class: 'col', part: 'bar',
        d: bar(left + 0.5, Math.min(top, zero), Math.max(right - left - 1, 1), Math.max(Math.abs(zero - top), 1), 'up', 2) })
      path.style.fill = hue(0)
      tips.bind(path, bin.label, bin.formatted)
      c.marks.append(path)
      if (m(bin.formatted, VALUE_PX) <= right - left + 4) {
        words.append(label((left + right) / 2, Math.min(top, zero) - 5, bin.formatted, 'value'))
      }
    }
    c.svg.append(words)
    if (!entered) c.svg.classList.add('enter')
    entered = true
    at.replaceChildren(c.svg)
  })
  return panel
}
