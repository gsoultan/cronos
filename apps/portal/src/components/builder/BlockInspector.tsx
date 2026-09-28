import type { ReactNode } from 'react'
import { Checkbox, MultiSelect, NumberInput, Select, TagsInput, TextInput } from '@mantine/core'
import { Field } from '../form/Field'
import { BasemapKeys } from './BasemapKeys'
import {
  basemapChoice, colours, DEFAULT_KEY, defaultStyle, drawnLayers, excludedBy, MAX_HEX_KM,
  readsPoints, readsShapes, relayer, shades, styleOptions, switchBasemap, takesSeries, type BasemapChoice,
} from '../../lib/maps'
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
  { value: 'line', label: 'Routes' },
  { value: 'hexbin', label: 'Hexagons' },
  { value: 'heat', label: 'Density' },
  { value: 'cluster', label: 'Clusters' },
  { value: 'bubble', label: 'Bubbles' },
  { value: 'scatter', label: 'Dots' },
  { value: 'flow', label: 'Flows' },
]

/* Shown whenever the rule has disabled a layer, so a greyed-out option comes
   with its reason rather than looking broken. */
const HEXAGONS_ALONE = 'Hexagons cannot share a map with shaded regions or routes: each '
  + 'shades from its own scale, and one legend cannot explain two.'

/** A layer as the picker names it, for the middle of a sentence. */
const layerName = (value: string) =>
  (LAYERS.find((l) => l.value === value)?.label ?? value).toLowerCase()

/** "a", "a and b", "a, b and c". */
function listed(items: string[]): string {
  return items.length < 2 ? items.join('') : `${items.slice(0, -1).join(', ')} and ${items.at(-1)}`
}

/* The three tile sources the server knows the terms of, between the default
   and the way out. None comes first because it is the default and stays so;
   custom tiles come last because they are for whatever is not named. */
const BASEMAPS: { value: BasemapChoice; label: string }[] = [
  { value: 'none', label: 'None' },
  { value: 'openstreetmap', label: 'OpenStreetMap' },
  { value: 'mapbox', label: 'Mapbox' },
  { value: 'google', label: 'Google Maps' },
  { value: 'url', label: 'Custom tiles' },
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

      {/* A map's rows are named by this unless its file names a label field
          of its own, `map.region`, which the builder keeps and does not offer.
          Then this only groups them, and saying "Labelled by" would name the
          wrong control. */}
      {(is(CATEGORICAL, block.kind) || is(PLOTS, block.kind) || block.kind === 'map'
        || block.kind === 'combo') && (
        <Field label={block.kind === 'map' && !block.map?.region ? 'Labelled by' : 'Grouped by'}
          help={groupHelp(block, fields)}>
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
        <MapFields block={block} dimensions={dimensions} onChange={onChange} />
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

function groupHelp({ kind, map }: Tile, fields: FieldDef[]): string {
  if (kind === 'map' && map?.region) {
    const label = fields.find((f) => f.name === map.region)?.label ?? map.region
    return `One region or point per value of this field. The file labels each one by ${label}.`
  }
  if (kind === 'map') return 'Names each region or point, in its tooltip and its legend.'
  if (is(PLOTS, kind)) return 'One dot per value of this field.'
  if (is(GRIDDED, kind)) return 'Along the top of the grid.'
  if (kind === 'waterfall') return 'One step per value, in this order.'
  return 'One bar, point or slice per value of this field.'
}

/**
 * The geography a map reads.
 *
 * Its own component because it is a dozen controls that only one kind shows,
 * and inlining them put the inspector's single `return` past the point anybody
 * could see which branch they were in.
 *
 * What shows is decided by what the server will draw rather than by the
 * picker's value. The two differ only when no layers are named and the fields
 * imply one, and then the fields are what the map is drawn from.
 */
function MapFields({ block, dimensions, onChange }: {
  block: Tile
  dimensions: FieldDef[]
  onChange: (patch: Partial<Tile>) => void
}) {
  const map = block.map ?? {}
  const set = (patch: Partial<TileMap>) => onChange({ map: { ...map, ...patch } })
  const layers = map.layers ?? []
  const drawn = drawnLayers(map)
  const excluded = excludedBy(layers)
  const implied = layers.length === 0 ? LAYERS.find((l) => l.value === drawn[0]) : undefined

  return (
    <>
      {/* Disabled rather than dropped on pick, so the author sees the rule
          before meeting it instead of watching a layer they chose vanish. */}
      <Field label="Layers" help={excluded.length > 0
        ? `Drawn bottom to top, in the order you pick them. ${HEXAGONS_ALONE}`
        : 'Drawn bottom to top, in the order you pick them.'}>
        <MultiSelect value={layers} clearable
          data={LAYERS.map(({ value, label }) => ({ value, label, disabled: excluded.includes(value) }))}
          placeholder={implied ? `${implied.label}, as the fields imply` : 'Choose what to draw'}
          onChange={(v) => onChange(relayer(block, v))} />
      </Field>

      {readsShapes(drawn) && (
        <Field label="Shapes"
          help="A field holding GeoJSON — ST_AsGeoJSON in PostGIS or DuckDB spatial. Regions and routes both read it.">
          <Select data={opts(dimensions)} value={map.geometry ?? null} allowDeselect={false}
            placeholder="Choose a field"
            onChange={(v) => set({ geometry: v ?? undefined })} />
        </Field>
      )}

      {readsPoints(drawn) && (
        <div className="grid grid-cols-2 gap-3">
          <Field label="Latitude">
            <Select data={opts(dimensions)} value={map.lat ?? null} allowDeselect={false}
              placeholder="Choose a field" onChange={(v) => set({ lat: v ?? undefined })} />
          </Field>
          <Field label="Longitude">
            <Select data={opts(dimensions)} value={map.lon ?? null} allowDeselect={false}
              placeholder="Choose a field" onChange={(v) => set({ lon: v ?? undefined })} />
          </Field>
        </div>
      )}

      {drawn.includes('flow') && (
        <div className="grid grid-cols-2 gap-3">
          <Field label="Ends at latitude">
            <Select data={opts(dimensions)} value={map.toLat ?? null} allowDeselect={false}
              placeholder="Choose a field" onChange={(v) => set({ toLat: v ?? undefined })} />
          </Field>
          <Field label="Ends at longitude">
            <Select data={opts(dimensions)} value={map.toLon ?? null} allowDeselect={false}
              placeholder="Choose a field" onChange={(v) => set({ toLon: v ?? undefined })} />
          </Field>
        </div>
      )}

      {/* A width, not a count across. A count redraws the grid whenever a
          filter moves the edge of the data, and a hexagon that changes size
          when somebody filters by carrier holds a number nobody can compare
          with the one before it. */}
      {drawn.includes('hexbin') && (
        <Field label="Hexagon width (km)" required={false}
          help="Flat side to flat side. Empty sizes them to the data, so a filter resizes them too.">
          <NumberInput value={map.hexKm ?? ''} min={0} max={MAX_HEX_KM} allowNegative={false}
            placeholder="Automatic" data-testid="hex-km"
            onChange={(v) => set({
              // Kept as typed, zero included: "0." is a number on its way to
              // 0.5, and clearing it would empty the field under the cursor.
              // The file writes zero as absent, which the server reads alike.
              hexKm: v === '' ? undefined : Number(v),
            })} />
        </Field>
      )}

      {shades(drawn) && <ShadeFields map={map} onChange={set} />}

      {takesSeries(drawn) && (
        <Field label="Coloured by" required={false}
          help="One colour per value, on every dot, bubble, cluster and flow.">
          <Select data={opts(dimensions)} value={block.series ?? null} clearable
            placeholder="One colour" data-testid="map-series"
            onChange={(v) => onChange({ series: v ?? undefined })} />
        </Field>
      )}

      {/* Said rather than left missing. Somebody who drew dots over shaded
          regions reasonably expects to colour the dots, and a control that is
          simply absent reads as a bug. Naming the layers says which one to
          move if the category matters more. */}
      {!takesSeries(drawn) && !drawn.every(colours) && (
        <p className="text-caption text-ink-muted">
          Colour already shows the value in {listed(drawn.filter(colours).map(layerName))}, so
          points here cannot also be coloured by a category. Give them a block of their own.
        </p>
      )}

      <BasemapFields map={map} onChange={set} />
    </>
  )
}

const CLASSES = [
  { value: 'quantile', label: 'The same number of places in each' },
  { value: 'equal', label: 'Equal steps of value' },
  { value: 'jenks', label: 'Where the values gap' },
  { value: 'custom', label: 'Breaks of my own' },
]

const RAMPS = [
  { value: 'sequential', label: 'One colour, darker for more' },
  { value: 'diverging', label: 'Two colours, either side of a middle' },
]

/**
 * How a shaded map's values become its six shades, and what the colours say.
 *
 * Quantile until changed, which suits the skewed measures maps mostly shade.
 * Custom breaks are somebody else's numbers — a regulator's thresholds, last
 * year's bands — so they are typed rather than dragged. Each control clears
 * what only it reads when it moves off, because the server refuses breaks
 * without custom classes, and a middle without two colours, rather than
 * ignoring them.
 */
function ShadeFields({ map, onChange }: {
  map: TileMap
  onChange: (patch: Partial<TileMap>) => void
}) {
  return (
    <>
      <Field label="Shades split" required={false} help="How the values are divided among the six shades.">
        <Select data={CLASSES} value={map.classify ?? 'quantile'} allowDeselect={false}
          data-testid="map-classify"
          onChange={(v) => onChange({
            classify: v && v !== 'quantile' ? v : undefined,
            breaks: v === 'custom' ? map.breaks : undefined,
          })} />
      </Field>
      {map.classify === 'custom' && (
        <Field label="Breaks" help="The top of each shade but the last. Up to five.">
          <TagsInput value={(map.breaks ?? []).map(String)} placeholder="Add a number"
            data-testid="map-breaks" onChange={(v) => onChange({ breaks: ascending(v) })} />
        </Field>
      )}
      <Field label="Colours" required={false}>
        <Select data={RAMPS} value={map.ramp ?? 'sequential'} allowDeselect={false}
          data-testid="map-ramp"
          onChange={(v) => onChange({
            ramp: v === 'diverging' ? v : undefined,
            midpoint: v === 'diverging' ? map.midpoint : undefined,
          })} />
      </Field>
      {map.ramp === 'diverging' && (
        <Field label="Middle" required={false} help="Where the two colours meet: a target, last year, zero.">
          <NumberInput value={map.midpoint ?? ''} placeholder="0" data-testid="map-midpoint"
            onChange={(v) => onChange({ midpoint: v === '' ? undefined : Number(v) })} />
        </Field>
      )}
    </>
  )
}

/** Typed breaks as the numbers the format takes: ascending, distinct, and no
 *  more than there are shades to put between. */
function ascending(typed: string[]): number[] {
  const n = typed.map((t) => Number(t.replace(/,/g, ''))).filter(Number.isFinite)
  return [...new Set(n)].sort((a, b) => a - b).slice(0, 5)
}

/**
 * What a map is drawn over.
 *
 * None by default, and it stays that way unless somebody picks one. A basemap
 * is a request from each reader's browser to a third party we would have
 * chosen for them, and it tells that party roughly where the data is.
 *
 * The key is asked for, and never written into the file. What the file holds
 * is a reference — the provider's default secret, or a `${secret:…}` a report
 * that bills a different account names, which this keeps without showing an
 * input for it — and BasemapKeys stores the value behind that reference on the
 * server. A key typed into the file itself would be in its history for ever.
 */
function BasemapFields({ map, onChange }: {
  map: TileMap
  onChange: (patch: Partial<TileMap>) => void
}) {
  const choice = basemapChoice(map)
  const styles = map.provider ? styleOptions(map.provider, map.style) : []

  return (
    <>
      <Field label="Basemap" required={false} help={basemapHelp(map)}>
        <Select data={BASEMAPS} value={choice} allowDeselect={false}
          data-testid="basemap-provider"
          onChange={(v) => onChange(switchBasemap((v ?? 'none') as BasemapChoice))} />
      </Field>

      {/* One option is not a choice, so OpenStreetMap shows none. */}
      {map.provider && styles.length > 1 && (
        <Field label="Style">
          <Select data={styles} value={map.style ?? defaultStyle(map.provider) ?? null}
            allowDeselect={false} data-testid="basemap-style"
            onChange={(v) => onChange({ style: v ?? undefined })} />
        </Field>
      )}

      {choice === 'url' && (
        <>
          <Field label="Tile URL" help="An https XYZ template, with {z}, {x} and {y} in it.">
            <TextInput value={map.basemap ?? ''} data-testid="basemap-url"
              placeholder="https://tile.example.org/{z}/{x}/{y}.png"
              classNames={{ input: 'font-mono text-caption' }}
              onChange={(e) => onChange({
                // Empty stays empty rather than becoming absent, which would
                // switch the basemap off under the cursor. See switchBasemap.
                basemap: e.currentTarget.value,
              })} />
          </Field>
          <Field label="Attribution" help="Every tile source requires its credit line be shown.">
            <TextInput value={map.attribution ?? ''} placeholder="© OpenStreetMap contributors"
              onChange={(e) => onChange({ attribution: e.currentTarget.value || undefined })} />
          </Field>
        </>
      )}

      <BasemapKeys map={map} />
    </>
  )
}

/**
 * What the chosen basemap costs and whom it asks, said before it is chosen:
 * otherwise each of these is found out from a bill, a letter or a blocked key.
 */
function basemapHelp(map: TileMap): ReactNode {
  // A key in the file is a report billing another account, and it is the one
  // worth naming. The default is the secret every map naming none reads — in
  // this project's secrets, or the server's environment behind them.
  const from = (secret: string) => (map.key
    ? <><code className="font-mono text-caption">{map.key}</code>, named in the file</>
    : <>the secret <code className="font-mono text-caption">{secret}</code></>)

  switch (map.provider) {
    case 'openstreetmap':
      return 'Needs no key. Its tile policy forbids heavy use: right for a demo or a quiet '
        + 'internal report, wrong for an embedded one at volume.'
    case 'mapbox':
      return (
        <>
          Reads its token from {from(DEFAULT_KEY.mapbox ?? '')}. Every reader’s browser
          receives it, so it has to be a public{' '}
          <code className="font-mono text-caption">pk.</code> token.
        </>
      )
    case 'google':
      return (
        <>
          Reads its key from {from(DEFAULT_KEY.google ?? '')}, which every reader’s
          browser receives — restrict it to the Map Tiles API. Google’s terms forbid its map
          beside another provider’s, so every map in this report has to use Google Maps or
          none; saving refuses a mix.
        </>
      )
  }
  if (map.basemap !== undefined) {
    return (
      <>
        Any XYZ tile server — your own, or one not named here. A key goes in the url as{' '}
        <code className="font-mono text-caption">{'${secret:tiles-…}'}</code>, never as itself.
      </>
    )
  }
  return 'None draws the data on its own. Anything else has each reader’s browser ask a '
    + 'third party for tiles, which tells it roughly where the data is.'
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
