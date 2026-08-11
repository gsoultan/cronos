import type { ReactNode } from 'react'
import { Button } from '@mantine/core'

/* One column, always. Two-column forms read faster to a designer and slower to
   everyone else — the eye has to guess whether to go right or down. */

export function FormSection({
  title, description, children,
}: { title: string; description?: string; children: ReactNode }) {
  return (
    <section className="mb-4 rounded-lg border border-line bg-surface p-6 shadow-card">
      <div className="mb-5">
        <h2 className="text-lead font-semibold text-ink">{title}</h2>
        {description && (
          <p className="mt-1 max-w-[68ch] text-small text-ink-secondary">{description}</p>
        )}
      </div>
      <div className="grid max-w-[640px] gap-5">{children}</div>
    </section>
  )
}

/** A section whose content needs the full width — the layout builder. */
export function FormSectionWide({
  title, description, children,
}: { title: string; description?: string; children: ReactNode }) {
  return (
    <section className="mb-4 rounded-lg border border-line bg-surface p-6 shadow-card">
      <div className="mb-5">
        <h2 className="text-lead font-semibold text-ink">{title}</h2>
        {description && (
          <p className="mt-1 max-w-[68ch] text-small text-ink-secondary">{description}</p>
        )}
      </div>
      {children}
    </section>
  )
}

interface ActionsProps {
  /** Disabled until something has actually changed. */
  canSubmit: boolean
  isSubmitting: boolean
  submitLabel: string
  onCancel: () => void
  /** Shown beside the buttons — what happens next, or what is blocking. */
  hint?: ReactNode
}

/**
 * Sticky footer so the primary action is reachable without scrolling to the
 * bottom of a long form, and so "am I done?" is answerable at a glance.
 */
export function FormActions({
  canSubmit, isSubmitting, submitLabel, onCancel, hint,
}: ActionsProps) {
  return (
    <div className="sticky bottom-0 z-20 mt-4 flex items-center justify-between gap-4
                    rounded-lg border border-line bg-surface px-6 py-4 shadow-pop">
      {hint && <p className="text-small text-ink-secondary">{hint}</p>}
      <div className="ml-auto flex gap-2">
        <Button variant="default" onClick={onCancel} disabled={isSubmitting}>Cancel</Button>
        <Button type="submit" disabled={!canSubmit} loading={isSubmitting}>{submitLabel}</Button>
      </div>
    </div>
  )
}

/** An aside inside a form: consequence, cost, or what happens next. */
export function Callout({ children }: { children: ReactNode }) {
  return (
    <p className="mt-3 max-w-[68ch] rounded-r-md border-l-2 border-accent bg-sunken
                  px-4 py-3 text-small text-ink-secondary">
      {children}
    </p>
  )
}
