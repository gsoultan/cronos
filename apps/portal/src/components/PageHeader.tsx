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
    /* Wraps rather than shrinks. Two buttons side by side do not fit at 390px,
       and `shrink-0` on the action group meant they pushed the document wide
       instead of dropping to their own line. */
    <header className="mb-6 flex flex-wrap items-start justify-between gap-x-6 gap-y-4">
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
      {actions && <div className="flex flex-wrap gap-2 max-sm:w-full">{actions}</div>}
    </header>
  )
}
