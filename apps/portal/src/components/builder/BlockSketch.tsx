import type { TileKind } from '../../lib/types'

/**
 * A representative drawing of a chart the canvas has no full component for.
 *
 * The canvas draws blocks with the components that will actually render them,
 * and for a bar, a line, a table and a number it still does. The rest — pies,
 * maps, funnels, treemaps — have no portal component, and the alternative was
 * what this replaces: `default: return null`, which drew nothing at all. An
 * author dropped a funnel on the canvas and got an empty cell.
 *
 * A sketch is not a preview and does not pretend to be: the shape is the chart
 * type's own, the proportions are invented, and the caption says so. What it
 * gives back is the thing the canvas is for — seeing where a block sits and
 * what it is, at the size it will occupy.
 */
export function BlockSketch({ kind, title }: { kind: TileKind; title: string }) {
  return (
    <div className="flex h-full flex-col overflow-hidden rounded-lg border border-line
                    bg-surface shadow-card">
      <div className="border-b border-line px-4 py-3">
        <h3 className="text-lead font-semibold text-ink">{title}</h3>
      </div>
      <div className="grid flex-1 place-items-center p-4">
        <svg viewBox="0 0 120 70" className="h-full max-h-[180px] w-full" aria-hidden>
          {shape(kind)}
        </svg>
      </div>
      <p className="px-4 pb-3 text-micro text-ink-muted">
        Sketch — drawn from your data when the report runs.
      </p>
    </div>
  )
}

const s = (n: number) => `var(--color-series-${n})`
const step = (n: number) => `var(--color-step-${n}, var(--color-series-1))`

/** Points around a middle, one per spoke from twelve o'clock: a radar's web. */
function spokes(reach: number[], r: number, cx = 60, cy = 36): string {
  return reach.map((k, i) => {
    const a = -Math.PI / 2 + (i * 2 * Math.PI) / reach.length
    return `${(cx + Math.cos(a) * r * k).toFixed(1)},${(cy + Math.sin(a) * r * k).toFixed(1)}`
  }).join(' ')
}

/** A bullet's shades, darkest furthest from its target. */
const shade = (k: number) => `color-mix(in srgb, var(--color-ink-muted) ${[34, 20, 9][k]}%, var(--color-surface))`

function shape(kind: TileKind) {
  switch (kind) {
    case 'bar':
      return [0.9, 0.66, 0.5, 0.3].map((w, i) => (
        <rect key={i} x="8" y={8 + i * 14} width={w * 104} height="10" rx="2" fill={s(1)} />
      ))

    case 'radar':
      return (
        <>
          {[1, 0.66, 0.33].map((k) => (
            <polygon key={k} points={spokes([k, k, k, k, k], 28)} fill="none"
              stroke="var(--color-line)" strokeWidth="1" />
          ))}
          <polygon points={spokes([0.9, 0.6, 0.8, 0.45, 0.7], 28)} fill={s(1)} fillOpacity="0.18"
            stroke={s(1)} strokeWidth="2" strokeLinejoin="round" />
        </>
      )

    case 'histogram':
      // Touching: the bins are one range, not a row of categories.
      return [10, 22, 38, 50, 44, 30, 18, 8].map((h, i) => (
        <rect key={i} x={8 + i * 13} y={62 - h} width="13" height={h} fill={s(1)}
          stroke="var(--color-surface)" strokeWidth="1" />
      ))

    case 'boxplot':
      return [[24, 18, 30, 40, 52, 58], [60, 10, 20, 28, 38, 46], [96, 22, 32, 36, 44, 54]].map(
        ([x = 0, hi = 0, q3 = 0, med = 0, q1 = 0, lo = 0]) => (
          <g key={x}>
            <line x1={x} x2={x} y1={hi} y2={lo} stroke="var(--color-ink-muted)" strokeWidth="1" />
            <rect x={x - 9} y={q3} width="18" height={q1 - q3} rx="2" fill={s(1)} fillOpacity="0.2"
              stroke={s(1)} strokeWidth="1.5" />
            <line x1={x - 9} x2={x + 9} y1={med} y2={med} stroke={s(1)} strokeWidth="2.5" />
          </g>
        ))

    case 'bullet':
      return [0.86, 0.52, 0.7].map((v, i) => (
        <g key={i}>
          {[[8, 58], [66, 30], [96, 16]].map(([x, w], k) => (
            <rect key={k} x={x} y={8 + i * 19} width={w} height="13" fill={shade(k)} />
          ))}
          <rect x="8" y={12.5 + i * 19} width={v * 104} height="4" rx="1" fill={s(1)} />
          <rect x="84" y={9 + i * 19} width="2" height="11" fill="var(--color-ink)" />
        </g>
      ))

    case 'pie':
    case 'donut':
      return (
        <>
          {/* Two arcs as stroked circles, the trick the viewer uses: a slice is
              a length of stroke-dasharray, so a pie and a donut differ only in
              how wide the stroke is. */}
          <circle cx="60" cy="35" r={kind === 'donut' ? 22 : 16}
            fill="none" stroke={s(1)} strokeWidth={kind === 'donut' ? 14 : 32}
            strokeDasharray={`${2 * Math.PI * (kind === 'donut' ? 22 : 16) * 0.62} 999`}
            transform="rotate(-90 60 35)" />
          <circle cx="60" cy="35" r={kind === 'donut' ? 22 : 16}
            fill="none" stroke={s(2)} strokeWidth={kind === 'donut' ? 14 : 32}
            strokeDasharray={`${2 * Math.PI * (kind === 'donut' ? 22 : 16) * 0.38} 999`}
            strokeDashoffset={`${-2 * Math.PI * (kind === 'donut' ? 22 : 16) * 0.62}`}
            transform="rotate(-90 60 35)" />
        </>
      )

    case 'scatter':
    case 'bubble':
      return [
        [24, 48, 4], [44, 30, 7], [62, 40, 5], [80, 18, 9], [96, 34, 4],
      ].map(([cx, cy, r], i) => (
        <circle key={i} cx={cx} cy={cy} r={kind === 'bubble' ? r : 4}
          fill={s((i % 3) + 1)} stroke="var(--color-surface)" strokeWidth="1" />
      ))

    case 'funnel':
      return [1, 0.72, 0.46, 0.26].map((w, i) => (
        <rect key={i} x={60 - (w * 100) / 2} y={6 + i * 16} width={w * 100} height="11"
          rx="2" fill={step(i + 1)} />
      ))

    case 'area':
      return (
        <>
          {/* Filled, which is the whole difference from a line — drawn as one
              it was a line chart with a different name on it. */}
          <polygon points="8,56 30,38 52,44 74,22 96,30 112,18 112,64 8,64"
            fill={s(1)} opacity="0.22" />
          <polyline points="8,56 30,38 52,44 74,22 96,30 112,18"
            fill="none" stroke={s(1)} strokeWidth="2" strokeLinejoin="round" />
        </>
      )

    case 'combo':
      return (
        <>
          {/* Bars *and* a line. Drawn with the bar component alone it was
              indistinguishable from a bar chart, which is the one thing a
              combo is not. */}
          {([[10, 34], [32, 24], [54, 40], [76, 18], [98, 28]] as const).map(([x, h], i) => (
            <rect key={i} x={x} y={64 - h} width="14" height={h} rx="1" fill={s(1)} />
          ))}
          <polyline points="17,26 39,34 61,16 83,30 105,12"
            fill="none" stroke={s(2)} strokeWidth="2" strokeLinejoin="round" />
        </>
      )

    case 'waterfall':
      return [
        [8, 40, 22], [28, 26, 14], [48, 20, 6], [68, 26, 18], [88, 10, 44],
      ].map(([x, y, h], i) => (
        <rect key={i} x={x} y={y} width="16" height={h} rx="1"
          fill={i === 4 ? 'var(--color-ink-muted)' : i === 3 ? s(8) : s(1)} />
      ))

    case 'heatmap':
      return Array.from({ length: 4 }, (_row, r) =>
        Array.from({ length: 6 }, (_col, c) => (
          <rect key={`${r}-${c}`} x={6 + c * 19} y={6 + r * 15} width="17" height="13" rx="1"
            fill={s(1)} opacity={0.15 + ((r * 6 + c) % 6) * 0.17} />
        )))

    case 'treemap':
      return [
        [4, 4, 62, 40], [4, 46, 62, 20], [68, 4, 48, 24], [68, 30, 48, 36],
      ].map(([x, y, w, h], i) => (
        <rect key={i} x={x} y={y} width={w} height={h} rx="1"
          fill={s(i + 1)} stroke="var(--color-surface)" strokeWidth="2" />
      ))

    case 'map':
      return (
        <>
          {/* A shape that reads as land without claiming to be anywhere. */}
          <path d="M14 46 L26 18 L48 12 L70 22 L92 14 L106 34 L96 58 L62 62 L34 56 Z"
            fill={s(1)} opacity="0.35" stroke={s(1)} strokeWidth="1" />
          <circle cx="46" cy="34" r="4" fill={s(2)} stroke="var(--color-surface)" strokeWidth="1" />
          <circle cx="82" cy="40" r="6" fill={s(2)} stroke="var(--color-surface)" strokeWidth="1" />
        </>
      )

    case 'gauge':
      return (
        <>
          {/* The figure belongs inside the dial. As a stat tile this was a
              number adrift in a card three times its height. */}
          <text x="60" y="47" textAnchor="middle" fontSize="15" fontWeight="650"
            fill="var(--color-ink)">68%</text>
          <circle cx="60" cy="42" r="26" fill="none" stroke="var(--color-line)" strokeWidth="9"
            strokeLinecap="round" strokeDasharray={`${2 * Math.PI * 26 * (240 / 360)} 999`}
            transform="rotate(150 60 42)" />
          <circle cx="60" cy="42" r="26" fill="none" stroke={s(1)} strokeWidth="9"
            strokeLinecap="round" strokeDasharray={`${2 * Math.PI * 26 * (240 / 360) * 0.68} 999`}
            transform="rotate(150 60 42)" />
        </>
      )

    default:
      // Every kind the canvas draws for real is handled before this file is
      // reached; anything else is a palette entry added without a sketch, and
      // a plain frame says so better than an empty cell.
      return <rect x="6" y="10" width="108" height="50" rx="3" fill="var(--color-grid)" />
  }
}
