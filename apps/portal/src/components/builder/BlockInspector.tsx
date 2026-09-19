import { Checkbox, MultiSelect, NumberInput, Select, TextInput } from '@mantine/core'
import { Field } from '../form/Field'
import { CATEGORICAL, FOLDED, GRIDDED, METERED, MULTI_SERIES, PLOTS, STACKABLE } from '../../lib/types'
import type {
  Dataset, Field as FieldDef, Tile, TileMap, TileMetric, TileTarget,
} from '../../lib/types'

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

/* Layers rather than a map type per combination. "Shade the regions and put a
   dot on each depot" is the question authors actually ask, and a type per
   combination is a list nobody can hold. */
const LAYERS = [
  { value: 'polygon', label: 'Shaded regions' },
  { value: 'heat', label: 'Density' },
  { value: 'bubble', label: 'Bubbles' },
  { value: 'scatter', label: 'Dots' },
  { value: 'flow', label: 'Flows' },
]

const is = (kinds: Tile['kind'][], kind: Tile['kind']) => kinds.includes(kind)

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

      {block.kind !== 'table' && !is(METERED, block.kind) && (
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

      {is(METERED, block.kind) && (
        <Metrics kind={block.kind} metrics={block.metrics ?? []} measures={measures}
          onChange={(metrics) => onChange({ metrics })} />
      )}

      {is(FOLDED, block.kind) && (
        <TargetField target={block.target ?? {}} measures={measures}
          onChange={(patch) => onChange({ target: { ...block.target, ...patch } })} />
      )}

      {(is(CATEGORICAL, block.kind) || is(PLOTS, block.kind) || block.kind === 'map'
        || block.kind === 'combo') && (
        <Field label={block.kind === 'map' ? 'Labelled by' : 'Grouped by'}
          help={groupHelp(block.kind)}>
          <Select data={opts(dimensions)} value={block.groupBy ?? null} allowDeselect={false}
            placeholder="Choose a field"
            onChange={(v) => onChange({ groupBy: v ?? undefined })} />
        </Field>
      )}

      {/* A scatter's horizontal axis is a number, not a bucket — which is the
          one place the shared "Grouped by" above means something different:
          there it names what each dot *is*, and this names where it sits. */}
      {is(PLOTS, block.kind) && (
        <Field label="Horizontal measure" help="The number on the bottom axis.">
          <Select data={opts(measures)} value={block.xField ?? null} allowDeselect={false}
            placeholder={measures.length ? 'Choose a measure' : 'This dataset has no measures'}
            disabled={measures.length === 0}
            onChange={(v) => onChange({ xField: v ?? undefined })} />
        </Field>
      )}

      {block.kind === 'bubble' && (
        <Field label="Sized by" help="Each bubble's area. Use a scatter if every dot is alike.">
          <Select data={opts(measures)} value={block.sizeField ?? null} allowDeselect={false}
            placeholder="Choose a measure"
            onChange={(v) => onChange({ sizeField: v ?? undefined })} />
        </Field>
      )}

      {is(MULTI_SERIES, block.kind) && (
        <Field label={is(GRIDDED, block.kind) ? 'Down the side' : 'Split by'}
          required={is(GRIDDED, block.kind)}
          help={seriesHelp(block.kind)}>
          <Select data={opts(dimensions)} value={block.series ?? null}
            clearable={!is(GRIDDED, block.kind)}
            placeholder={is(GRIDDED, block.kind) ? 'Choose a field' : 'One series'}
            onChange={(v) => onChange({
              series: v ?? undefined,
              // A stack of one series is the same drawing, so dropping the
              // split drops the stacking with it rather than leaving a
              // setting that quietly does nothing.
              stacked: v ? block.stacked : undefined,
            })} />
        </Field>
      )}

      {is(STACKABLE, block.kind) && block.series && (
        <Checkbox label="Stack the series" checked={block.stacked ?? false}
          onChange={(e) => onChange({ stacked: e.currentTarget.checked || undefined })} />
      )}

      {block.kind === 'map' && (
        <MapFields map={block.map ?? {}} dimensions={dimensions}
          onChange={(patch) => onChange({ map: { ...block.map, ...patch } })} />
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

function seriesHelp(kind: Tile['kind']): string {
  if (is(GRIDDED, kind)) return 'The other axis of the grid. A heatmap needs both.'
  if (kind === 'treemap') return 'Groups the rectangles, drawing one box per value.'
  return 'Draws one series per value of this field.'
}

/**
 * The measures a combo or a funnel reads.
 *
 * A list rather than one field, because that is what these charts are: a combo
 * is bars and a line against the same buckets, and a funnel's stages are very
 * often separate columns rather than rows of a stage dimension.
 */
function Metrics({ kind, metrics, measures, onChange }: {
  kind: Tile['kind']
  metrics: TileMetric[]
  measures: FieldDef[]
  onChange: (metrics: TileMetric[]) => void
}) {
  const combo = kind === 'combo'
  const at = (i: number, patch: Partial<TileMetric>) =>
    onChange(metrics.map((m, j) => (j === i ? { ...m, ...patch } : m)))

  return (
    <Field label={combo ? 'Measures' : 'Stages'} data-testid="metrics"
      help={combo
        ? 'Drawn together against the same buckets.'
        : 'One stage per measure, in this order.'}>
      <div className="grid gap-2">
        {metrics.map((m, i) => (
          <div key={`${m.field}-${i}`} className="grid gap-1.5 rounded-md border border-line p-2">
            <div className="flex items-center gap-2">
              <Select data={opts(measures)} value={m.field || null} allowDeselect={false}
                className="min-w-0 flex-1" aria-label={`Measure ${i + 1}`}
                placeholder="Choose a measure"
                onChange={(v) => at(i, { field: v ?? '' })} />
              <Select data={AGGREGATES} value={m.aggregate ?? 'sum'} allowDeselect={false} w={120}
                aria-label={`Measure ${i + 1} summarised as`}
                onChange={(v) => at(i, { aggregate: (v ?? 'sum') as TileMetric['aggregate'] })} />
              <button type="button" aria-label={`Remove measure ${i + 1}`}
                onClick={() => onChange(metrics.filter((_, j) => j !== i))}
                className="shrink-0 cursor-pointer text-small text-ink-muted underline">Remove</button>
            </div>
            <TextInput size="xs" value={m.label ?? ''} placeholder="Label (optional)"
              aria-label={`Measure ${i + 1} label`}
              onChange={(e) => at(i, { label: e.currentTarget.value || undefined })} />
            {combo && (
              <div className="flex items-center gap-3">
                <Select data={DRAWS} value={m.draw ?? 'bar'} allowDeselect={false} w={110}
                  aria-label={`Measure ${i + 1} drawn as`}
                  onChange={(v) => at(i, { draw: (v ?? 'bar') as TileMetric['draw'] })} />
                {/* Reachable, and never a default. Two scales on one plot is
                    the most-flagged mistake in charting: where they line up is
                    a choice nobody made, so the chart shows a correlation that
                    is not in the data. */}
                <Checkbox size="xs" label="Own scale" checked={m.secondary ?? false}
                  onChange={(e) => at(i, { secondary: e.currentTarget.checked || undefined })} />
              </div>
            )}
          </div>
        ))}
        <button type="button" data-testid="add-metric"
          disabled={measures.length === 0}
          onClick={() => onChange([...metrics, {
            field: measures[0]?.name ?? '', aggregate: 'sum',
            draw: combo ? (metrics.length === 0 ? 'bar' : 'line') : undefined,
          }])}
          className="cursor-pointer justify-self-start text-small text-ink-muted underline
                     hover:text-ink disabled:cursor-default disabled:opacity-50">
          Add a measure
        </button>
      </div>
    </Field>
  )
}

/** What a gauge reads its value against: a column, or a fixed number. */
function TargetField({ target, measures, onChange }: {
  target: TileTarget
  measures: FieldDef[]
  onChange: (patch: Partial<TileTarget>) => void
}) {
  const fixed = target.value !== undefined
  return (
    <>
      <Field label="Compared against">
        {/* Both are ordinary: "this month's quota" is a column, and "95%" is a
            number somebody agreed once and does not want a table for. */}
        <Select data={TARGETS} value={fixed ? 'value' : 'field'} allowDeselect={false}
          onChange={(v) => onChange(v === 'value'
            ? { value: 0, field: undefined }
            : { value: undefined, field: measures[0]?.name })} />
      </Field>
      {fixed ? (
        <Field label="Target">
          <NumberInput value={target.value ?? 0} data-testid="target-value"
            onChange={(v) => onChange({ value: typeof v === 'number' ? v : Number(v) || 0 })} />
        </Field>
      ) : (
        <Field label="Target measure">
          <Select data={opts(measures)} value={target.field ?? null} allowDeselect={false}
            placeholder="Choose a measure"
            onChange={(v) => onChange({ field: v ?? undefined })} />
        </Field>
      )}
      <Field label="Target label" required={false}>
        <TextInput value={target.label ?? ''} placeholder="Target"
          onChange={(e) => onChange({ label: e.currentTarget.value || undefined })} />
      </Field>
    </>
  )
}

const DRAWS = [
  { value: 'bar', label: 'Bars' },
  { value: 'line', label: 'Line' },
]

const TARGETS = [
  { value: 'field', label: 'A measure' },
  { value: 'value', label: 'A fixed number' },
]

function groupHelp(kind: Tile['kind']): string {
  if (kind === 'map') return 'Names each region or point, in its tooltip and its legend.'
  if (is(PLOTS, kind)) return 'One dot per value of this field.'
  if (is(GRIDDED, kind)) return 'Along the top of the grid.'
  if (kind === 'waterfall') return 'One step per value, in this order.'
  return 'One bar, point or slice per value of this field.'
}

/**
 * The geography a map reads.
 *
 * Its own component because it is six controls that only one kind shows, and
 * inlining them put the inspector's single `return` past the point anybody
 * could see which branch they were in.
 */
function MapFields({ map, dimensions, onChange }: {
  map: TileMap
  dimensions: FieldDef[]
  onChange: (patch: Partial<TileMap>) => void
}) {
  const layers = map.layers ?? []
  const needsPoints = layers.some((l) => l !== 'polygon')

  return (
    <>
      <Field label="Layers" help="Drawn bottom to top, in the order you pick them.">
        <MultiSelect data={LAYERS} value={layers} clearable
          placeholder="Choose what to draw"
          onChange={(v) => onChange({ layers: v })} />
      </Field>

      {layers.includes('polygon') && (
        <Field label="Region shapes"
          help="A field holding GeoJSON — ST_AsGeoJSON in PostGIS or DuckDB spatial.">
          <Select data={opts(dimensions)} value={map.geometry ?? null} allowDeselect={false}
            placeholder="Choose a field"
            onChange={(v) => onChange({ geometry: v ?? undefined })} />
        </Field>
      )}

      {needsPoints && (
        <div className="grid grid-cols-2 gap-3">
          <Field label="Latitude">
            <Select data={opts(dimensions)} value={map.lat ?? null} allowDeselect={false}
              placeholder="Choose a field" onChange={(v) => onChange({ lat: v ?? undefined })} />
          </Field>
          <Field label="Longitude">
            <Select data={opts(dimensions)} value={map.lon ?? null} allowDeselect={false}
              placeholder="Choose a field" onChange={(v) => onChange({ lon: v ?? undefined })} />
          </Field>
        </div>
      )}

      {layers.includes('flow') && (
        <div className="grid grid-cols-2 gap-3">
          <Field label="Ends at latitude">
            <Select data={opts(dimensions)} value={map.toLat ?? null} allowDeselect={false}
              placeholder="Choose a field" onChange={(v) => onChange({ toLat: v ?? undefined })} />
          </Field>
          <Field label="Ends at longitude">
            <Select data={opts(dimensions)} value={map.toLon ?? null} allowDeselect={false}
              placeholder="Choose a field" onChange={(v) => onChange({ toLon: v ?? undefined })} />
          </Field>
        </div>
      )}

      {/* Empty by default, and it stays that way unless somebody types a URL.
          A basemap is a request from the reader's browser to a third party we
          would have chosen for them, and on OpenStreetMap's own servers it is
          against the tile usage policy at any volume. */}
      <Field label="Basemap tiles" required={false}
        help="An XYZ template. Leave empty to draw the data on its own.">
        <TextInput value={map.basemap ?? ''} data-testid="basemap-url"
          placeholder="https://tile.example.org/{z}/{x}/{y}.png"
          classNames={{ input: 'font-mono text-caption' }}
          onChange={(e) => onChange({ basemap: e.currentTarget.value || undefined })} />
      </Field>

      {map.basemap && (
        <Field label="Attribution" help="Every tile source requires its credit line be shown.">
          <TextInput value={map.attribution ?? ''} placeholder="© OpenStreetMap contributors"
            onChange={(e) => onChange({ attribution: e.currentTarget.value || undefined })} />
        </Field>
      )}
    </>
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
