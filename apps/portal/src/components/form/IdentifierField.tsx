import { useState } from 'react'
import { TextInput } from '@mantine/core'
import { Field } from './Field'

interface Props {
  value: string
  /** Called only on a deliberate edit — the caller stops syncing from the name. */
  onChange: (value: string) => void
  onBlur?: () => void
  error?: string
  /** Rendered before the value so it reads as the thing it becomes. */
  prefix?: string
  /** One line: who points at this. */
  usedFor: string
}

/**
 * The machine-readable name, kept out of the way until someone wants it.
 *
 * It was a full-weight text input on four different forms, each explaining
 * itself differently, sitting at the same visual weight as Name — which reads
 * as a decision you have to make. It is not: it is derived, almost nobody
 * should change it, and the people who do are looking for it.
 *
 * So it collapses to one muted line showing the value in the place it will
 * actually appear, and the consequence of changing it is stated at the moment
 * of changing it rather than in help text nobody reads beforehand.
 */
export function IdentifierField({ value, onChange, onBlur, error, prefix, usedFor }: Props) {
  const [open, setOpen] = useState(false)

  // A validation error must never be hidden behind a disclosure.
  if (!open && !error) {
    return (
      <div className="flex items-baseline gap-2 text-caption">
        <span className="shrink-0 text-ink-muted">API name</span>
        {/* No prefix here: in a 320px rail it eats the value, and the value is
            the part worth checking. The path appears when you open the field,
            which is exactly when the context matters. */}
        <code className="min-w-0 truncate font-mono text-ink-secondary" title={value}>
          {value || '…'}
        </code>
        <button type="button" onClick={() => setOpen(true)}
          className="ml-auto shrink-0 cursor-pointer text-ink-muted underline">
          Change
        </button>
      </div>
    )
  }

  return (
    <Field label="API name" error={error} help={usedFor}>
      {/* The prefix sits beside the input rather than inside it. Mantine's
          leftSection needs an explicit pixel width, and guessing one against a
          mono font clips the value — layout gets it right without arithmetic. */}
      <div className="flex items-center gap-1.5">
        {prefix && (
          <span className="shrink-0 font-mono text-caption text-ink-muted">{prefix}</span>
        )}
        <TextInput value={value} onBlur={onBlur} autoFocus={open} className="min-w-0 flex-1"
          classNames={{ input: 'font-mono' }}
          onChange={(e) => onChange(e.currentTarget.value)} />
      </div>
      <p className="mt-1 text-caption text-ink-muted">
        Anything already pointing at this will stop resolving. Safe to change
        before anyone uses it.
      </p>
    </Field>
  )
}
