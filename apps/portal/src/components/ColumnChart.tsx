import { useState } from 'react'
import { ChartFrame } from './ChartFrame'
import { ChartTooltip, TooltipRow } from './ChartTooltip'
import { axisTick, currency } from '../lib/format'
import { SERIES } from '../theme/theme'
import { useMeasure } from '../lib/useMeasure'
import { niceTicks } from '../lib/scale'

export interface ColumnDatum {
  /** What the band is called. A month, a customer, a status — the caller says. */
  label: string
  [series: string]: string | number
}

interface Props {
  title: string
  subtitle?: string
  data: ColumnDatum[]
  series: readonly string[]
  format?: (n: number) => string
  /*
     How to write a band's label.

     Identity by default, and that is the important half. This axis used to run
     every label through monthLabel — the datum's key was `month`, so a bar
     chart was assumed to be over time. Group a real report by customer and the
     labels arrive as "c-1", "c-2", "c-3"; `new Date("c-1")` is not an error in
     JavaScript, it is the 1st of January 2001. So the chart quietly relabelled
     three customers as three months and drew it with a straight face.

     Anything a lenient parser accepts was at risk: "12" became December, and a
     status or a region survived only because it happened to be unparseable.

     A live report needs no formatter at all — the engine already wrote "May
     2026", because it is the thing that knew the grain. Only the sample view,
     whose fixture holds ISO dates, asks for one.
  */
  labelText?: (label: string) => string
}

const H = 280
const PAD = { top: 12, right: 12, bottom: 32, left: 56 }
const MAX_BAR = 24   // cap it — never fill the slot; the leftover is air
const GAP = 2        // surface gap between stacked segments
const CAP = 4        // rounded data-end, square at the baseline

/** Stacked columns: part-to-whole over time. */
export function ColumnChart({ title, subtitle, data, series, format = currency,
  labelText = (l) => l }: Props) {
  const [ref, width] = useMeasure<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)

  const totals = data.map((d) => series.reduce((s, k) => s + (d[k] as number), 0))
  const ticks = niceTicks(Math.max(...totals), 4)
  const scaleMax = ticks.at(-1)!

  const plotW = Math.max(0, width - PAD.left - PAD.right)
  const plotH = H - PAD.top - PAD.bottom
  const y = (v: number) => PAD.top + plotH - (v / scaleMax) * plotH
  const band = data.length ? plotW / data.length : 0
  const barW = Math.min(MAX_BAR, band * 0.6)
  const cx = (i: number) => PAD.left + band * i + band / 2

  const legend = series.map((k, i) => ({ label: k, color: SERIES[i]! }))

  return (
    <ChartFrame title={title} subtitle={subtitle} legend={legend}>
      <div className="relative" ref={ref} onMouseLeave={() => setHover(null)}>
        {width > 0 && (
          <svg width={width} height={H} role="img" aria-label={title} className="block overflow-visible">
            {ticks.map((t) => (
              <g key={t}>
                <line x1={PAD.left} x2={width - PAD.right} y1={y(t)} y2={y(t)} strokeWidth={1}
                  stroke={t === 0 ? 'var(--color-baseline)' : 'var(--color-grid)'} />
                <text x={PAD.left - 10} y={y(t) + 4} textAnchor="end"
                  className="chart-axis-text tabular-nums">{axisTick(t)}</text>
              </g>
            ))}

            {data.map((d, i) => {
              let acc = 0
              const segs = series.map((k, si) => {
                const v = d[k] as number
                const y0 = y(acc)
                acc += v
                return { k, si, y0, y1: y(acc), top: si === series.length - 1 }
              })
              return (
                <g key={d.label} onMouseEnter={() => setHover(i)}
                  opacity={hover === null || hover === i ? 1 : 0.4}>
                  <rect x={PAD.left + band * i} y={PAD.top} width={band} height={plotH}
                    fill="transparent" />
                  {segs.map(({ k, si, y0, y1, top }) => (
                    <path key={k} fill={SERIES[si]}
                      d={roundedTop(cx(i) - barW / 2, y1, barW, Math.max(0, y0 - y1 - GAP), top ? CAP : 0)} />
                  ))}
                  <text x={cx(i)} y={H - 12} textAnchor="middle" className="chart-axis-text">
                    {labelText(d.label)}
                  </text>
                </g>
              )
            })}
          </svg>
        )}

        {hover !== null && width > 0 && (
          <ChartTooltip x={cx(hover)} y={y(totals[hover]!) - 12}
            heading={labelText(data[hover]!.label)}>
            {series.map((k, si) => (
              <TooltipRow key={k} color={SERIES[si]} label={k}
                value={format(data[hover]![k] as number)} />
            ))}
            <TooltipRow label="Total" value={format(totals[hover]!)} total />
          </ChartTooltip>
        )}
      </div>
    </ChartFrame>
  )
}

/** Rounded at the data-end, square at the baseline. */
function roundedTop(x: number, y: number, w: number, h: number, r: number): string {
  if (h <= 0) return ''
  const rr = Math.min(r, h, w / 2)
  if (rr <= 0) return `M${x},${y}h${w}v${h}h${-w}Z`
  return `M${x},${y + rr}a${rr},${rr} 0 0 1 ${rr},${-rr}h${w - 2 * rr}` +
    `a${rr},${rr} 0 0 1 ${rr},${rr}v${h - rr}h${-w}Z`
}
