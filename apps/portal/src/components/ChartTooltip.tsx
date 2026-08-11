import type { ReactNode } from 'react'

interface Props {
  /** Positioned against the plot, anchored bottom-centre on the mark. */
  x: number
  y: number
  heading: string
  children: ReactNode
}

export function ChartTooltip({ x, y, heading, children }: Props) {
  return (
    <div role="tooltip"
      style={{ left: x, top: y, transform: 'translate(-50%, -100%)' }}
      className="pointer-events-none absolute z-10 min-w-[170px] rounded-md border
                 border-line bg-surface px-3 py-2 text-small shadow-pop">
      <p className="mb-1 text-caption font-semibold text-ink-muted">{heading}</p>
      {children}
    </div>
  )
}

export function TooltipRow({
  color, label, value, total,
}: { color?: string; label: string; value: string; total?: boolean }) {
  return (
    <p className={`my-0.5 flex items-center gap-2 ${
      total ? 'mt-2 border-t border-line pt-2 font-semibold text-ink' : 'text-ink-secondary'
    }`}>
      {color && <span className="size-2 shrink-0 rounded-[2px]" style={{ background: color }} />}
      {label}
      <span className="ml-auto font-medium text-ink tabular-nums">{value}</span>
    </p>
  )
}
