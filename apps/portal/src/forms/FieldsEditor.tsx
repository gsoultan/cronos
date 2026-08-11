import { Select, Switch, TextInput } from '@mantine/core'
import type { Field, FieldType } from '../lib/types'

interface Props {
  fields: Field[]
  onChange: (fields: Field[]) => void
}

const TYPES: { value: FieldType; label: string }[] = [
  { value: 'string', label: 'Text' },
  { value: 'number', label: 'Whole number' },
  { value: 'decimal', label: 'Decimal' },
  { value: 'date', label: 'Date' },
  { value: 'bool', label: 'Yes / no' },
  { value: 'enum', label: 'One of a set' },
]

const FORMATS = [
  { value: '', label: 'Plain' },
  { value: 'currency', label: 'Money' },
  { value: 'percent', label: 'Percentage' },
  { value: 'number', label: 'Number with separators' },
]

/**
 * The dataset's contract with everything downstream.
 *
 * This is what separates a governed dataset from a saved query. The label is
 * what a report author picks from a list; the role decides whether a field can
 * be summarised or grouped by; the type decides which filter operators are
 * even offered. Get these right once here and nobody has to think about them
 * again.
 */
export function FieldsEditor({ fields, onChange }: Props) {
  const set = (i: number, patch: Partial<Field>) =>
    onChange(fields.map((f, j) => (j === i ? { ...f, ...patch } : f)))

  if (fields.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-line bg-sunken px-6 py-10
                    text-center text-small text-ink-secondary">
        Fields appear here once the query returns columns.
      </p>
    )
  }

  return (
    <div className="overflow-hidden rounded-lg border border-line">
      <div className="grid grid-cols-[1fr_1fr_140px_150px_140px_90px] gap-3 border-b border-line
                      bg-sunken px-4 py-2 text-micro font-semibold tracking-[0.04em]
                      text-ink-secondary uppercase">
        <span>Column</span><span>Label</span><span>Type</span>
        <span>Role</span><span>Format</span><span>Hidden</span>
      </div>
      {fields.map((f, i) => (
        <div key={f.name}
          className="grid grid-cols-[1fr_1fr_140px_150px_140px_90px] items-center gap-3
                     border-b border-line px-4 py-2 last:border-b-0 hover:bg-hover">
          <span className="truncate font-mono text-caption text-ink-secondary">{f.name}</span>

          <TextInput size="xs" value={f.label} aria-label={`Label for ${f.name}`}
            onChange={(e) => set(i, { label: e.currentTarget.value })} />

          <Select size="xs" data={TYPES} value={f.type} allowDeselect={false}
            aria-label={`Type of ${f.name}`}
            onChange={(v) => set(i, { type: (v ?? 'string') as FieldType })} />

          <Select size="xs" allowDeselect={false} aria-label={`Role of ${f.name}`}
            data={[
              { value: 'dimension', label: 'Group or filter by' },
              { value: 'measure', label: 'Summarise' },
            ]}
            value={f.role} onChange={(v) => set(i, { role: (v ?? 'dimension') as Field['role'] })} />

          <Select size="xs" data={FORMATS} value={f.format ?? ''} allowDeselect={false}
            aria-label={`Format of ${f.name}`}
            disabled={f.role !== 'measure'}
            onChange={(v) => set(i, { format: (v || undefined) as Field['format'] })} />

          {/* Hidden fields stay usable by row-level security and joins, but are
              never offered to a report author — the place to put an id. */}
          <Switch size="xs" checked={!!f.hidden} aria-label={`Hide ${f.name}`}
            onChange={(e) => set(i, { hidden: e.currentTarget.checked })} />
        </div>
      ))}
    </div>
  )
}
