import { el } from './dom'
import { slotOf } from './palette'

/** How tall a plotted chart is drawn, in pixels. */
export const PLOT_HEIGHT = 240

/** A chart's panel: its heading, then whatever draws it. */
export function chartPanel(title: string, wide = true): HTMLElement {
  return el('section', { class: wide ? 'panel wide' : 'panel', part: 'panel' }, el('h3', {}, title))
}

/** A chart with nothing to draw says so, where the chart would be. */
export function nothing(panel: HTMLElement, text = 'No data in this period.'): HTMLElement {
  panel.append(el('p', { class: 'unaffected' }, text))
  return panel
}

/** A series' colour, folded into the palette's last slot past its end. */
export const hue = (slot: number, size?: number) => `var(--cr-series-${slotOf(slot, size)})`

/** Where a chart draws, sized to its panel. */
export function host(panel: HTMLElement): HTMLElement {
  const at = el('div', { class: 'chart-host' })
  panel.append(at)
  return at
}
