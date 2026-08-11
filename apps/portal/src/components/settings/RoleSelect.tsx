import { Select } from '@mantine/core'

interface Option {
  value: string
  label: string
  hint: string
}

interface Props {
  value: string
  options: Option[]
  onChange: (value: string) => void
  /** Disables the whole control with a stated reason. */
  lockedReason?: string
  /** Disables specific options, each with its own reason. */
  disable?: (value: string) => string | undefined
  width?: number
  label?: string
}

/**
 * A role, changed in place.
 *
 * Roles were static tags, which meant the only way to change someone's access
 * was to remove and re-invite them. They are a single field with a small set of
 * values, so they take a select and save on change — a form with a save button
 * around one dropdown is ceremony.
 *
 * Options that would break a rule are disabled *with the reason attached*.
 * Letting someone pick "Member" on the last owner and only then refusing has
 * already misled them.
 */
export function RoleSelect({
  value, options, onChange, lockedReason, disable, width = 150, label,
}: Props) {
  return (
    <Select
      size="xs" w={width} allowDeselect={false} value={value} aria-label={label ?? 'Role'}
      disabled={!!lockedReason}
      title={lockedReason}
      onChange={(v) => v && onChange(v)}
      data={options.map((o) => {
        const reason = disable?.(o.value)
        return {
          value: o.value,
          label: reason ? `${o.label} — ${reason}` : o.label,
          disabled: !!reason,
        }
      })}
    />
  )
}
