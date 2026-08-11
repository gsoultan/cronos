import { MultiSelect, Select, TextInput } from '@mantine/core'
import { Field } from '../form/Field'
import type { Dataset, Field as FieldDef, Tile } from '../../lib/types'

interface Props {
  block: Tile
  /** Fields of whichever dataset this block reads. */
  fields: FieldDef[]
  /** Every dataset in the project, so a block can read somewhere else. */
  datasets: Dataset[]
  /** The report's default, shown as the fallback option. */
  defaultDataset: string
  onChange: (patch: Partial<Tile>) => void
}

const AGGREGATES = [
  { value: 'sum', label: 'Total' },
  { value: 'avg', label: 'Average' },
  { value: 'count', label: 'Count' },
  { value: 'min', label: 'Lowest' },
  { value: 'max', label: 'Highest' },
]

const opts = (fs: FieldDef[]) => fs.map((f) => ({ value: f.name, label: f.label }))

/** Twelve columns, expressed as the four splits anyone actually wants. */
const WIDTHS = [
  { span: 3, label: 'Quarter', bars: 1 },
  { span: 6, label: 'Half', bars: 2 },
  { span: 9, label: 'Three quarters', bars: 3 },
  { span: 12, label: 'Full', bars: 4 },
]

/**
 * Moving a block to another dataset re-seeds its fields from the new one rather
 * than clearing them. The old choices are genuinely invalid — they named columns
 * that no longer exist — but blanking them leaves a block that looks broken, and
 * a sensible default is one click away from whatever the author actually wants.
 */
function rebind(
  block: Tile, next: string | null, defaultDataset: string, datasets: Dataset[],
): Partial<Tile> {
  const name = !next || next === defaultDataset ? undefined : next
  const target = datasets.find((d) => d.name === (name ?? defaultDataset))
  const usable = target?.fields.filter((f) => !f.hidden) ?? []
  return {
    dataset: name,
    field: usable.find((f) => f.role === 'measure')?.name,
    groupBy: block.kind === 'stat' ? undefined : usable.find((f) => f.role === 'dimension')?.name,
    columns: block.kind === 'table' ? usable.slice(0, 5).map((f) => f.name) : undefined,
  }
}

export function BlockInspector({
  block, fields, datasets, defaultDataset, onChange,
}: Props) {
  const visible = fields.filter((f) => !f.hidden)
  const measures = visible.filter((f) => f.role === 'measure')
  const dimensions = visible.filter((f) => f.role === 'dimension')

  return (
    <div className="grid gap-4">
      <Field label="Title">
        <TextInput value={block.title} onChange={(e) => onChange({ title: e.currentTarget.value })} />
      </Field>

      {/* Per-block, so one report can combine invoices and shipments. Changing
          it clears the field choices, which belonged to the old dataset — a
          silently invalid reference is worse than an obvious reset. */}
      <Field label="Reads from"
        help={block.dataset && block.dataset !== defaultDataset
          ? 'This block reads a different dataset from the rest of the report.'
          : 'Uses the report’s dataset unless you change it.'}>
        <Select allowDeselect={false} value={block.dataset ?? defaultDataset}
          data={datasets.map((d) => ({
            value: d.name,
            label: d.name === defaultDataset ? `${d.label} (report default)` : d.label,
          }))}
          onChange={(v) => onChange(rebind(block, v, defaultDataset, datasets))} />
      </Field>

      {/* A picture of the width, not a number of columns. Nobody thinks
          "span 9"; they think "three quarters of the row". */}
      <Field label="Width">
        <div className="grid grid-cols-4 gap-1.5" role="radiogroup" aria-label="Width">
          {WIDTHS.map((w) => (
            <button key={w.span} type="button" role="radio" aria-checked={block.span === w.span}
              title={w.label} onClick={() => onChange({ span: w.span })}
              className={`grid cursor-pointer gap-1 rounded-md border p-2 hover:border-accent
                ${block.span === w.span ? 'border-accent bg-accent-wash' : 'border-line'}`}>
              <span aria-hidden className="flex h-3 gap-px">
                {[0, 1, 2, 3].map((i) => (
                  <span key={i} className={`flex-1 rounded-[1px] ${
                    i < w.bars ? 'bg-accent' : 'bg-grid'}`} />
                ))}
              </span>
              <span className="text-micro leading-tight text-ink-secondary">{w.label}</span>
            </button>
          ))}
        </div>
      </Field>

      {block.kind !== 'table' && (
        <>
          <Field label="Measure" help="The number this block is about.">
            <Select data={opts(measures)} value={block.field ?? null} allowDeselect={false}
              placeholder={measures.length ? 'Choose a measure' : 'This dataset has no measures'}
              disabled={measures.length === 0}
              onChange={(v) => onChange({ field: v ?? undefined })} />
          </Field>
          <Field label="Summarised as">
            <Select data={AGGREGATES} value={block.aggregate ?? 'sum'} allowDeselect={false}
              onChange={(v) => onChange({ aggregate: (v ?? 'sum') as Tile['aggregate'] })} />
          </Field>
        </>
      )}

      {(block.kind === 'bar' || block.kind === 'line') && (
        <Field label="Grouped by" help="One bar or point per value of this field.">
          <Select data={opts(dimensions)} value={block.groupBy ?? null} allowDeselect={false}
            placeholder="Choose a field"
            onChange={(v) => onChange({ groupBy: v ?? undefined })} />
        </Field>
      )}

      {block.kind === 'table' && (
        <Field label="Columns" help="Shown left to right in the order you pick them.">
          <MultiSelect data={opts(visible)} value={block.columns ?? []} searchable clearable
            placeholder="Choose columns"
            onChange={(v) => onChange({ columns: v })} />
        </Field>
      )}

      {block.kind === 'table' && (
        <Field label="Sorted by" required={false}
          help="The first key decides the order; the rest break ties.">
          <div className="grid gap-2">
            {(block.sort ?? []).map((key, i) => (
              <div key={`${key.field}-${i}`} className="flex items-center gap-2">
                <Select data={opts(visible)} value={key.field} allowDeselect={false}
                  className="min-w-0 flex-1" aria-label={`Sort key ${i + 1}`}
                  onChange={(v) => onChange({ sort: replace(block.sort, i, { ...key, field: v ?? key.field }) })} />
                <Select data={DIRECTIONS} value={key.dir ?? 'asc'} allowDeselect={false} w={130}
                  aria-label={`Sort direction ${i + 1}`}
                  onChange={(v) => onChange({ sort: replace(block.sort, i, { ...key, dir: (v ?? 'asc') as 'asc' | 'desc' }) })} />
                <button type="button" aria-label={`Remove sort key ${i + 1}`}
                  onClick={() => onChange({ sort: without(block.sort, i) })}
                  className="shrink-0 cursor-pointer text-small text-ink-muted underline">Remove</button>
              </div>
            ))}
            <button type="button" data-testid="add-sort"
              disabled={visible.length === 0}
              onClick={() => onChange({
                sort: [...(block.sort ?? []), { field: visible[0]?.name ?? '', dir: 'desc' as const }],
              })}
              className="cursor-pointer justify-self-start text-small text-ink-muted underline
                         hover:text-ink disabled:cursor-default disabled:opacity-50">
              Add a sort key
            </button>
          </div>
        </Field>
      )}

      {/* SQL, because the format takes SQL and the server compiles it against
          the dataset — which is what returns a sentence naming the field when
          it is wrong. A field-operator-value row here would refuse every
          predicate that is not one comparison, which is most of the ones worth
          writing. */}
      <Field label="Only rows where" required={false}
        help="Narrows this block alone. Checked against the dataset when you save.">
        <TextInput value={block.filter ?? ''} placeholder="status = 'overdue'"
          classNames={{ input: 'font-mono text-caption' }} data-testid="block-filter"
          onChange={(e) => onChange({ filter: e.currentTarget.value || undefined })} />
      </Field>

      <p className="rounded-r-md border-l-2 border-line bg-sunken px-3 py-2 text-caption
                    text-ink-muted">
        Drag to reorder, or <kbd className="font-mono">⌥↑</kbd> /{' '}
        <kbd className="font-mono">⌥↓</kbd>. <kbd className="font-mono">Delete</kbd> removes
        this block.
      </p>
    </div>
  )
}

const DIRECTIONS = [
  { value: 'asc', label: 'Ascending' },
  { value: 'desc', label: 'Descending' },
]

/** One key changed, the rest untouched. */
function replace(keys: Tile['sort'], at: number, key: NonNullable<Tile['sort']>[number]) {
  return (keys ?? []).map((k, i) => (i === at ? key : k))
}

function without(keys: Tile['sort'], at: number) {
  return (keys ?? []).filter((_, i) => i !== at)
}
