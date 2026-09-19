import { el } from './dom'
import { svg, n } from './svg'
import type { Axis } from './types'

/**
 * The frame a plotted chart is drawn in: gridlines, axes and tick labels.
 *
 * Tick *labels* are HTML and the marks are SVG, which is the one structural
 * decision here. An SVG that stretches to its panel stretches its text with
 * it, so the same chart renders 9px type in a sidebar and 20px type across a
 * dashboard; keeping labels outside means they wear the host's font at the
 * host's size wherever the panel ends up.
 *
 * Coordinates inside the SVG run 0..100 across and 0..height down, with y
 * already flipped — callers work in fractions and never think about it.
 */
export interface Plot {
  frame: HTMLElement
  canvas: SVGSVGElement
  /** Places a fraction pair (0..1, origin bottom-left) in SVG coordinates. */
  at(fx: number, fy: number): [number, number]
}

export const PLOT_W = 100

/**
 * @param stretch true to fill the panel's width, distorting the coordinate
 * space. Right for a line, whose marks are strokes that can carry
 * `non-scaling-stroke`; wrong for anything round, which would become an
 * ellipse at every width but one.
 */
export function plot(x: Axis | null, y: Axis | null, height: number, stretch: boolean): Plot {
  const canvas = svg('svg', {
    viewBox: `0 0 ${PLOT_W} ${height}`,
    preserveAspectRatio: stretch ? 'none' : 'xMidYMid meet',
    class: 'canvas',
    part: 'chart',
    // Decorative: the marks inside carry their own labels, and the block's
    // heading names the whole. A reader on a screen reader gets the numbers
    // from the marks rather than "graphic" from the container.
    'aria-hidden': 'true',
  })

  // Gridlines stay in the SVG. They are strokes, so they scale without
  // distorting, and they belong behind the marks in one stacking context.
  for (const t of y?.ticks ?? []) {
    canvas.append(svg('line', {
      class: 'grid', x1: 0, x2: PLOT_W, y1: n((1 - t.at) * height), y2: n((1 - t.at) * height),
    }))
  }

  const frame = el('div', { class: 'plot' },
    el('div', { class: 'ys' }, ...(y?.ticks ?? []).map((t) =>
      el('span', { style: `bottom:${t.at * 100}%` }, t.label))),
    el('div', { class: 'canvas-wrap' }, canvas),
    el('div', { class: 'xs' }, ...(x?.ticks ?? []).map((t) =>
      el('span', { style: `left:${t.at * 100}%` }, t.label))))

  return { frame, canvas, at: (fx, fy) => [fx * PLOT_W, (1 - fy) * height] }
}

/** Where v sits on a, as a fraction from 0 at min to 1 at max. */
export function on(a: Axis, v: number): number {
  return a.max === a.min ? 0 : (v - a.min) / (a.max - a.min)
}
