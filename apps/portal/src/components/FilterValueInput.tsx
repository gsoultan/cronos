import { NumberInput, MultiSelect, Select, TextInput, Group } from '@mantine/core'
import type { Field } from '../lib/types'
import type { OperatorSpec } from '../lib/operators'
import { RELATIVE_UNITS } from '../lib/operators'

interface Props {
  field: Field
  spec: OperatorSpec
  value: unknown
  onChange: (v: unknown) => void
}

/**
 * The value editor changes shape with the operator, so the control always
 * matches what is being asked for. Nobody types a date into a text box here.
 */
export function FilterValueInput({ field, spec, value, onChange }: Props) {
  switch (spec.input) {
    case 'none':
      return null

    case 'multi':
      return (
        <MultiSelect
          data={field.values ?? []}
          value={Array.isArray(value) ? (value as string[]) : []}
          onChange={onChange}
          placeholder="Choose one or more"
          searchable
          clearable
          w={260}
        />
      )

    case 'number':
      return (
        <NumberInput
          value={typeof value === 'number' ? value : ''}
          onChange={(v) => onChange(v === '' ? undefined : Number(v))}
          placeholder="0"
          thousandSeparator=","
          w={160}
        />
      )

    case 'numberRange': {
      const [lo, hi] = Array.isArray(value) ? (value as (number | undefined)[]) : []
      return (
        <Group gap={8} wrap="nowrap">
          <NumberInput value={lo ?? ''} placeholder="From" thousandSeparator="," w={120}
            onChange={(v) => onChange([v === '' ? undefined : Number(v), hi])} />
          <span className="text-small text-ink-muted">and</span>
          <NumberInput value={hi ?? ''} placeholder="To" thousandSeparator="," w={120}
            onChange={(v) => onChange([lo, v === '' ? undefined : Number(v)])} />
        </Group>
      )
    }

    case 'date':
      return (
        <TextInput type="date" w={170}
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.currentTarget.value)} />
      )

    case 'dateRange': {
      const [from, to] = Array.isArray(value) ? (value as (string | undefined)[]) : []
      return (
        <Group gap={8} wrap="nowrap">
          <TextInput type="date" w={150} value={from ?? ''}
            onChange={(e) => onChange([e.currentTarget.value, to])} />
          <span className="text-small text-ink-muted">and</span>
          <TextInput type="date" w={150} value={to ?? ''}
            onChange={(e) => onChange([from, e.currentTarget.value])} />
        </Group>
      )
    }

    /* "in the last [30] [days]" — resolved against the run date, not today. */
    case 'relative': {
      const v = (value ?? { n: 30, unit: 'day' }) as { n: number; unit: string }
      return (
        <Group gap={8} wrap="nowrap">
          <NumberInput value={v.n} min={1} w={90}
            onChange={(n) => onChange({ ...v, n: Number(n) || 1 })} />
          <Select data={[...RELATIVE_UNITS]} value={v.unit} w={130} allowDeselect={false}
            onChange={(unit) => onChange({ ...v, unit: unit ?? 'day' })} />
        </Group>
      )
    }

    default:
      return (
        <TextInput w={220} placeholder="Type a value"
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.currentTarget.value)} />
      )
  }
}
