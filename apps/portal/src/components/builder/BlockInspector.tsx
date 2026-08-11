import { MultiSelect, Select, TextInput } from '@mantine/core'
import { Field } from '../form/Field'
import type { Field as FieldDef, Tile } from '../../lib/types'

interface Props {
  block: Tile
  fields: FieldDef[]
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

export function BlockInspector({ block, fields, onChange }: Props) {
  const visible = fields.filter((f) => !f.hidden)
  const measures = visible.filter((f) => f.role === 'measure')
  const dimensions = visible.filter((f) => f.role === 'dimension')

  return (
    <div className="grid gap-4">
      <Field label="Title">
        <TextInput value={block.title} onChange={(e) => onChange({ title: e.currentTarget.value })} />
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

      <p className="rounded-r-md border-l-2 border-line bg-sunken px-3 py-2 text-caption
                    text-ink-muted">
        Drag to reorder, or <kbd className="font-mono">⌥↑</kbd> /{' '}
        <kbd className="font-mono">⌥↓</kbd>. <kbd className="font-mono">Delete</kbd> removes
        this block.
      </p>
    </div>
  )
}
