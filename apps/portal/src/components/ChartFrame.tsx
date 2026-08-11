import type { ReactNode } from 'react'
import { Legend, type LegendItem } from './Legend'

interface Props {
  title: string
  subtitle?: string
  /** A legend is always present for two or more series, never for one. */
  legend?: LegendItem[]
  action?: ReactNode
  children: ReactNode
}

/**
 * The shell every chart sits in: title, optional legend, chart surface.
 * Keeping it in one place is what makes twelve charts look like one system.
 */
export function ChartFrame({ title, subtitle, legend, action, children }: Props) {
  return (
    <figure className="m-0 rounded-lg border border-line bg-surface p-4 shadow-card">
      <div className="flex items-start justify-between gap-3">
        <div>
          <figcaption className="text-lead font-semibold text-ink">{title}</figcaption>
          {subtitle && <p className="mt-1 text-small text-ink-secondary">{subtitle}</p>}
        </div>
        {action}
      </div>
      {legend && legend.length > 1 && <Legend items={legend} />}
      <div className="mt-3">{children}</div>
    </figure>
  )
}
