import type { ReactNode } from 'react'

interface Props {
  title: string
  /** Say what to do next, never just what is missing. */
  description: string
  action?: ReactNode
}

export function EmptyState({ title, description, action }: Props) {
  return (
    <div className="rounded-lg border border-dashed border-line bg-surface px-6 py-16 text-center">
      <p className="text-lead font-semibold text-ink">{title}</p>
      <p className="mx-auto mt-2 max-w-[46ch] text-ink-secondary">{description}</p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}
