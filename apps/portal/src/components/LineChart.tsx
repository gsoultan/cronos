import { useState } from 'react'
import { ChartFrame } from './ChartFrame'
import { ChartTooltip, TooltipRow } from './ChartTooltip'
import { monthLabel } from '../lib/format'
import { useMeasure } from '../lib/useMeasure'
import { niceBand } from '../lib/scale'

interface Point { month: string; value: number }

interface Props {
  title: string
  subtitle?: string
  data: Point[]
  format?: (n: number) => string
  /** Optional reference line — a target or threshold. */
  target?: { value: number; label: string }
}

const H = 280
const PAD = { top: 12, right: 56, bottom: 32, left: 56 }

/**
 * Single-series trend. One series means no legend box — the title already says
 * what is plotted — and the value is direct-labelled at the end of the line.
 */
export function LineChart({ title, subtitle, data, format = (n) => `${n}%`, target }: Props) {
  const [ref, width] = useMeasure<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)

  const values = data.map((d) => d.value)
  const ticks = niceBand(
    Math.min(...values, target?.value ?? Infinity),
    Math.max(...values, target?.value ?? -Infinity),
    4,
  )
  const lo = ticks[0]!
  const hi = ticks.at(-1)!

  const plotW = Math.max(0, width - PAD.left - PAD.right)
  const plotH = H - PAD.top - PAD.bottom
  const x = (i: number) => PAD.left + (data.length > 1 ? (plotW / (data.length - 1)) * i : plotW / 2)
  const y = (v: number) => PAD.top + plotH - ((v - lo) / (hi - lo || 1)) * plotH

  const d = data.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(i)},${y(p.value)}`).join(' ')
  const last = data.at(-1)!

  function onMove(e: React.MouseEvent<SVGSVGElement>) {
    if (!plotW || data.length < 2) return
    const rect = e.currentTarget.getBoundingClientRect()
    const rel = e.clientX - rect.left - PAD.left
    const i = Math.round((rel / plotW) * (data.length - 1))
    setHover(Math.max(0, Math.min(data.length - 1, i)))
  }

  return (
    <ChartFrame title={title} subtitle={subtitle}>
      <div className="relative" ref={ref}>
        {width > 0 && (
          <svg width={width} height={H} role="img" aria-label={title}
            className="block overflow-visible"
            onMouseMove={onMove} onMouseLeave={() => setHover(null)}>
            {ticks.map((t) => (
              <g key={t}>
                <line x1={PAD.left} x2={width - PAD.right} y1={y(t)} y2={y(t)}
                  stroke="var(--color-grid)" strokeWidth={1} />
                <text x={PAD.left - 10} y={y(t) + 4} textAnchor="end"
                  className="chart-axis-text tabular-nums">{format(t)}</text>
              </g>
            ))}

            {target && (
              <>
                <line x1={PAD.left} x2={width - PAD.right} y1={y(target.value)} y2={y(target.value)}
                  stroke="var(--color-baseline)" strokeWidth={1} />
                <text x={width - PAD.right + 6} y={y(target.value) + 4}
                  className="chart-axis-text">{target.label}</text>
              </>
            )}

            <path d={d} fill="none" stroke="var(--color-series-1)" strokeWidth={2}
              strokeLinecap="round" strokeLinejoin="round" />

            {/* End marker with a 2px surface ring, plus the one direct label. */}
            <circle cx={x(data.length - 1)} cy={y(last.value)} r={4.5}
              fill="var(--color-series-1)" stroke="var(--color-surface)" strokeWidth={2} />
            <text x={x(data.length - 1) + 10} y={y(last.value) + 4}
              className="chart-value-label tabular-nums">{format(last.value)}</text>

            {data.map((p, i) => (
              <text key={p.month} x={x(i)} y={H - 12} textAnchor="middle"
                className="chart-axis-text">{monthLabel(p.month)}</text>
            ))}

            {hover !== null && (
              <>
                <line x1={x(hover)} x2={x(hover)} y1={PAD.top} y2={PAD.top + plotH}
                  stroke="var(--color-baseline)" strokeWidth={1} />
                <circle cx={x(hover)} cy={y(data[hover]!.value)} r={4.5}
                  fill="var(--color-series-1)" stroke="var(--color-surface)" strokeWidth={2} />
              </>
            )}
          </svg>
        )}

        {hover !== null && width > 0 && (
          <ChartTooltip x={x(hover)} y={y(data[hover]!.value) - 12}
            heading={monthLabel(data[hover]!.month)}>
            <TooltipRow color="var(--color-series-1)" label={title}
              value={format(data[hover]!.value)} />
          </ChartTooltip>
        )}
      </div>
    </ChartFrame>
  )
}
