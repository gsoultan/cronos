import type { ReactNode } from 'react'

/** A titled surface. Used for the results table and every settings block. */
export function Panel({
  title, subtitle, meta, action, children, flush,
}: {
  title: string
  subtitle?: string
  meta?: ReactNode
  action?: ReactNode
  children?: ReactNode
  /** Content sits edge to edge — tables and lists supply their own padding. */
  flush?: boolean
}) {
  return (
    <section className="mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card">
      <div className="flex items-center justify-between gap-4 border-b border-line p-4">
        <div>
          <h2 className="text-lead font-semibold text-ink">{title}</h2>
          {subtitle && <p className="mt-1 text-small text-ink-secondary">{subtitle}</p>}
        </div>
        {meta && <span className="text-small text-ink-muted">{meta}</span>}
        {action}
      </div>
      {children && <div className={flush ? '' : 'p-4'}>{children}</div>}
    </section>
  )
}
