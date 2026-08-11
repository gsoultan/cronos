import { useId, type ReactNode } from 'react'

interface Props {
  label: string
  /** Sits between label and input: read before the mistake, not after. */
  help?: ReactNode
  error?: string
  required?: boolean
  children: ReactNode
}

/**
 * The one field wrapper. Every input in the app is wrapped in this, so labels,
 * help and errors sit in the same place on every form and nobody has to learn a
 * second layout.
 *
 * Optional fields are marked, not required ones. Most fields on most forms are
 * required, so marking those is noise; marking the exceptions is information.
 */
export function Field({ label, help, error, required = true, children }: Props) {
  const labelId = useId()
  const helpId = useId()

  /* The label is associated, not merely adjacent. A Field can hold two controls
     (a range, a prefix plus an input), so this is a labelled group rather than
     a <label> wrapping one input — a wrapper would silently mislabel the second
     control. Screen readers announce the group name for everything inside it. */
  return (
    <div className={`grid gap-1 ${error ? 'field-invalid' : ''}`}>
      <div className="flex items-baseline gap-2">
        <span id={labelId} className="text-small font-semibold text-ink">{label}</span>
        {!required && (
          <span className="text-micro tracking-[0.04em] text-ink-muted uppercase">Optional</span>
        )}
      </div>
      {help && (
        <p id={helpId} className="mb-1 max-w-[62ch] text-small text-ink-secondary">{help}</p>
      )}
      <div role="group" aria-labelledby={labelId}
        aria-describedby={help ? helpId : undefined} className="min-w-0">
        {children}
      </div>
      {error && (
        <p role="alert" className="flex items-baseline gap-1 text-small text-delta-bad">
          <span aria-hidden>⚠</span> {error}
        </p>
      )}
    </div>
  )
}

/**
 * Pulls the first error out of a TanStack field, but only once the field has
 * been touched — validating as someone types tells them they are wrong before
 * they have finished being right.
 */
export function fieldError(meta: {
  isTouched: boolean
  errors: unknown[]
}): string | undefined {
  if (!meta.isTouched) return undefined
  const first = meta.errors[0]
  if (!first) return undefined
  return typeof first === 'string' ? first : String((first as { message?: string }).message ?? first)
}
