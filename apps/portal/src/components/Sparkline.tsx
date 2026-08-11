interface Props {
  values: number[]
  width?: number
  height?: number
}

/** 12-point trend in the de-emphasis hue, current point in the accent. */
export function Sparkline({ values, width = 96, height = 28 }: Props) {
  if (values.length < 2) return null

  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  const stepX = width / (values.length - 1)
  const y = (v: number) => height - 3 - ((v - min) / span) * (height - 6)

  const d = values.map((v, i) => `${i === 0 ? 'M' : 'L'}${i * stepX},${y(v)}`).join(' ')
  const last = values.at(-1)!

  return (
    <svg width={width} height={height} aria-hidden className="shrink-0">
      <path d={d} fill="none" stroke="var(--color-series-muted)" strokeWidth={2}
        strokeLinecap="round" strokeLinejoin="round" />
      {/* Surface ring keeps the end dot legible where it crosses the line. */}
      <circle cx={width} cy={y(last)} r={4} fill="var(--color-accent)"
        stroke="var(--color-surface)" strokeWidth={2} />
    </svg>
  )
}
