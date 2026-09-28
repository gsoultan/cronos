import { Select, TagsInput, TextInput } from '@mantine/core'
import { Field } from '../form/Field'
import type { Dataset, ReportFilter } from '../../lib/types'
import { toIdentifier } from '../../lib/validators'

interface Props {
  filters: ReportFilter[]
  /** Every dataset the report's blocks actually read, in the order they were
   *  introduced — the report's default first. */
  datasets: Dataset[]
  onChange: (filters: ReportFilter[]) => void
}

const TYPES = [
  { value: 'date', label: 'Date' },
  { value: 'enum', label: 'One of a list' },
  { value: 'string', label: 'Text' },
  { value: 'number', label: 'Number' },
  { value: 'bool', label: 'Yes or no' },
  { value: 'area', label: 'A place on a map' },
]

/**
 * The report's shared filters.
 *
 * A filter spans blocks that may read different datasets, so it says what it
 * means in each — that is the `bind` map, and it is a field picker per dataset
 * rather than one picker, because a dataset left unbound is a legitimate
 * outcome the viewer shows on the block. "Period" has nothing to say to a
 * dataset of current stock levels, and the interface is required to say so
 * rather than let it be discovered.
 */
export function FiltersEditor({ filters, datasets, onChange }: Props) {
  const at = (i: number, patch: Partial<ReportFilter>) =>
    onChange(filters.map((f, j) => (j === i ? { ...f, ...patch } : f)))

  return (
    <div className="grid gap-3" data-testid="report-filters">
      {filters.length === 0 && (
        <p className="text-caption text-ink-muted">
          No filters. A report without them shows the same numbers to everybody
          who opens it.
        </p>
      )}

      {filters.map((f, i) => (
        <div key={i} className="grid gap-2 rounded-md border border-line p-3">
          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-[140px] flex-1">
              <Field label="Shown as">
              <TextInput size="xs" value={f.label ?? ''} placeholder="Period"
                aria-label={`Filter ${i + 1} label`}
                onChange={(e) => {
                  const label = e.currentTarget.value
                  // The name is the contract the embed token and the host page
                  // bind to, so it follows the label only until somebody has
                  // typed one — after that renaming it would break their page.
                  at(i, f.name ? { label } : { label, name: toIdentifier(label) })
                }} />
              </Field>
            </div>
            <div className="w-[150px]">
              <Field label="Type">
              <Select size="xs" data={TYPES} value={f.type} allowDeselect={false}
                aria-label={`Filter ${i + 1} type`}
                onChange={(v) => at(i, retyped(f, v ?? 'string'))} />
              </Field>
            </div>
            <button type="button" aria-label={`Remove filter ${i + 1}`}
              onClick={() => onChange(filters.filter((_, j) => j !== i))}
              className="cursor-pointer pb-1.5 text-small text-ink-muted underline">
              Remove
            </button>
          </div>

          <Field label="Name" help="What a host page sets. Changing it breaks their code.">
            <TextInput size="xs" value={f.name} placeholder="period"
              aria-label={`Filter ${i + 1} name`}
              classNames={{ input: 'font-mono text-caption' }}
              onChange={(e) => at(i, { name: toIdentifier(e.currentTarget.value) })} />
          </Field>

          {f.type === 'enum' && (
            <Field label="Values" help="The only things it can be set to.">
              <TagsInput size="xs" value={f.values ?? []} placeholder="Add a value"
                aria-label={`Filter ${i + 1} values`}
                onChange={(v) => at(i, { values: v })} />
            </Field>
          )}

          <Field label="Narrows"
            help="A dataset left blank is unaffected, and each block says so.">
            <div className="grid gap-1.5">
              {datasets.map((d) => (
                <div key={d.name} className="flex items-center gap-2">
                  <span className="w-[120px] shrink-0 truncate text-caption text-ink-secondary">
                    {d.label}
                  </span>
                  {f.type === 'area' ? (
                    <PlaceFields dataset={d} value={f.bind[d.name]}
                      label={`Filter ${i + 1} in ${d.label}`}
                      onChange={(v) => at(i, { bind: rebound(f.bind, d.name, v) })} />
                  ) : (
                    <Select size="xs" className="min-w-0 flex-1" clearable
                      placeholder="Not affected"
                      aria-label={`Filter ${i + 1} in ${d.label}`}
                      data={d.fields.map((x) => ({ value: x.name, label: x.label }))}
                      value={f.bind[d.name] ?? null}
                      onChange={(v) => at(i, { bind: rebound(f.bind, d.name, v) })} />
                  )}
                </div>
              ))}
            </div>
          </Field>
        </div>
      ))}

      <button type="button" data-testid="add-filter"
        disabled={datasets.length === 0}
        onClick={() => onChange([...filters, { name: '', type: 'date', bind: {} }])}
        className="cursor-pointer justify-self-start text-small text-ink-muted underline
                   hover:text-ink disabled:cursor-default disabled:opacity-50">
        Add a filter
      </button>
    </div>
  )
}

/**
 * A filter given another type.
 *
 * Its control goes: a control suits some types and not others, and checkboxes
 * left on a date is a save the server refuses. So does what it narrows, when
 * the change is to or from an area — a field where a pair of them belongs, or
 * the other way round, is a binding that means nothing.
 */
function retyped(f: ReportFilter, type: string): Partial<ReportFilter> {
  const place = (t: string) => t === 'area'
  return {
    type, control: undefined,
    bind: place(type) === place(f.type) ? f.bind : {},
  }
}

/** The bind map with one dataset's entry set, or taken out when there is none. */
function rebound(bind: Record<string, string>, dataset: string, v: string | null) {
  const next = { ...bind }
  if (v) next[dataset] = v
  else delete next[dataset]
  return next
}

/**
 * Where a dataset keeps its places, for an area to narrow it by.
 *
 * Two pickers where every other type has one, because a place is two columns;
 * written as the pair the format takes, `lat,lon`. Half a pair is written as
 * it stands and refused on save, with a sentence showing what it holds — the
 * same as a filter bound to nothing, rather than an author's choice undone.
 */
function PlaceFields({ dataset, value, label, onChange }: {
  dataset: Dataset
  value: string | undefined
  label: string
  onChange: (pair: string | null) => void
}) {
  const [lat = '', lon = ''] = (value ?? '').split(',').map((s) => s.trim())
  const numbers = dataset.fields
    .filter((x) => x.type === 'number' || x.type === 'decimal')
    .map((x) => ({ value: x.name, label: x.label }))
  const set = (a: string, b: string) => onChange(a || b ? `${a},${b}` : null)
  return (
    <div className="flex min-w-0 flex-1 gap-1.5">
      <Select size="xs" className="min-w-0 flex-1" clearable placeholder="Latitude"
        aria-label={`${label}, latitude`} data={numbers} value={lat || null}
        onChange={(v) => set(v ?? '', lon)} />
      <Select size="xs" className="min-w-0 flex-1" clearable placeholder="Longitude"
        aria-label={`${label}, longitude`} data={numbers} value={lon || null}
        onChange={(v) => set(lat, v ?? '')} />
    </div>
  )
}
