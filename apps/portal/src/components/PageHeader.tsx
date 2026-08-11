import type { ReactNode } from 'react'

interface Props {
  eyebrow?: string
  title: string
  description?: string
  /** Exactly one primary action per page — the obvious next step. */
  actions?: ReactNode
}

export function PageHeader({ eyebrow, title, description, actions }: Props) {
  return (
    <header className="mb-6 flex items-start justify-between gap-6">
      <div>
        {eyebrow && (
          <p className="mb-1 text-caption font-medium tracking-[0.06em] text-ink-muted uppercase">
            {eyebrow}
          </p>
        )}
        <h1 className="text-display font-semibold tracking-tight text-ink">{title}</h1>
        {description && (
          <p className="mt-2 max-w-[60ch] text-ink-secondary">{description}</p>
        )}
      </div>
      {actions && <div className="flex shrink-0 gap-2">{actions}</div>}
    </header>
  )
}
