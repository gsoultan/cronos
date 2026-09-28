import { StatTile } from './StatTile'
import { ServerChart } from './ServerChart'
import { DataTable } from './DataTable'
import { Panel } from './Panel'
import { EmptyState } from './EmptyState'
import type { ReportBlock, ReportView } from '../lib/api'
import type { Bar, DrawOptions, MapViewer, TableBlock } from '@cronos/charts'
import type { Field } from '../lib/types'

/**
 * A report as the server computed it.
 *
 * The blocks arrive already aggregated and already formatted — the engine that
 * knew the currency did the formatting, so the same figure reads identically
 * here, in an embedded widget and in the PDF of the same report. Nothing on
 * this page recomputes anything; if a number looks wrong it is wrong in one
 * place.
 *
 * Deliberately built from the same components the sample view uses. Two
 * renderings of a stat tile would drift, and the one nobody looks at would be
 * the one a customer sees.
 */
export function LiveReport({ view, applied = [], mapView, filter }: {
  view: ReportView
  /* How a map sets one of the report's filters — a region clicked, a view —
     or, with null, lets it go. */
  filter?: DrawOptions['filter']
  /* How a large map asks for the part a reader has in view: this page's own
     route and credentials, which the renderer never holds. */
  mapView?: MapViewer
  /*
     Which filters the reader has actually set.

     The coverage hint below is a property of the report — this block does not
     read that filter — and it is only *information* once somebody has filtered
     and a number has not moved. Shown unconditionally it was four identical
     captions on an unfiltered screen, naming a filter the page did not even
     offer a control for.
  */
  applied?: string[]
}) {
  const stats = view.blocks.filter((b) => b.kind === 'stat')
  const rest = view.blocks.filter((b) => b.kind !== 'stat')

  return (
    <div className="grid gap-4" data-testid="live-report">
      {/* auto-fit against the column this report is drawn in, not against the
          window. `sm:` and `xl:` are viewport widths, and the width that
          decides how many tiles fit is this column's — which changes when the
          navigation collapses, and is a customer's page in the embed. At 1192px
          those two spellings differed by two tiles a row.

          The track stops growing at 320px rather than taking 1fr. A row of
          these is read by scanning across it, and a report with two stats was
          giving each of them half of a 1560px column — a 48px figure alone in
          a box wide enough for a paragraph, which reads as something missing
          rather than as a headline. Capped, two stats and six stats are the
          same tile, and the tile is the width StatTile sizes its hero figure
          against.

          320 and not 380 because auto-fit counts repetitions against the
          track's maximum, not its minimum: at 380 a 728px column fitted one
          track and stacked two tiles that had room to sit side by side.

          `min(…, 100%)` on both ends of the track, and min-w-0 on the row. A
          fixed 380px maximum is wider than a phone, and a track maximum does
          not shrink to fit: at 390px it laid a 380px tile in a 358px column
          and pushed the whole document 50px wide, which scrolls the page
          sideways on every screen this is supposed to fit. min-w-0 is the
          other half — these rows are grid items themselves, and an item's
          automatic minimum size is its content's, so the 380 and the 400 below
          would otherwise set a floor for the column above them. */}
      {stats.length > 0 && (
        <div className="grid min-w-0 gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(220px,100%),min(320px,100%)))]">
          {stats.map((b, i) => (
            <div key={b.title + i}>
              <StatTile label={b.title} value={b.value ?? '—'} hero={i === 0} />
              <Unaffected block={b} view={view} applied={applied} />
            </div>
          ))}
        </div>
      )}
      {rowsOf(rest).map((row, i) => (
        <div key={i}
          className="grid min-w-0 items-stretch gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(400px,100%),1fr))]">
          {row.map((b, j) => (
            <Block key={b.title + j} block={b} view={view} applied={applied} mapView={mapView}
              filter={filter} />
          ))}
        </div>
      ))}
    </div>
  )
}

/**
 * The blocks, grouped into the rows they can share.
 *
 * Every block used to get a row of its own, so a chart with three bars was
 * drawn at the same width as a table with three columns and the page was a
 * column of cards with nothing to compare across. Two charts side by side is
 * the reading this format is for.
 *
 * A table takes its own row rather than half of one. Its columns have minimum
 * widths that do not shrink — that is what stops a measure column collapsing
 * to nothing — so half a row is where it starts scrolling sideways instead.
 *
 * Grouped rather than left to grid auto-placement with a span, because
 * auto-placement would leave a hole beside every table and the only way to
 * backfill it is `grid-auto-flow: dense`, which moves a later block above an
 * earlier one. The author chose this order; a layout is not entitled to
 * rewrite it to save a gap.
 */
export function rowsOf(blocks: ReportBlock[]): ReportBlock[][] {
  const out: ReportBlock[][] = []
  // The row still taking blocks, or none — a table closes it and opens nothing.
  let open: ReportBlock[] | null = null
  for (const b of blocks) {
    if (b.kind === 'table') {
      out.push([b])
      open = null
    } else if (open) {
      open.push(b)
    } else {
      open = [b]
      out.push(open)
    }
  }
  return out
}

function Block({ block, view, applied, mapView, filter }: {
  block: ReportBlock
  view: ReportView
  applied: string[]
  mapView?: MapViewer
  filter?: DrawOptions['filter']
}) {
  if (block.kind === 'chart') {
    /*
     * Drawn by @cronos/charts — the same renderers the embed uses.
     *
     * This was `block.chart !== 'bar'`, and everything else came back as
     * "line charts need a newer portal" for a report the server had rendered
     * perfectly. There were three renderers of this payload and the quiet one
     * drew one chart type out of fourteen; the message even blamed the portal
     * for it, which was at least honest.
     */
    return (
      <div className="flex flex-col">
        <div className="min-h-0 flex-1">
          <ServerChart block={block} mapView={mapView} filter={filter} />
        </div>
        <Unaffected block={block} view={view} applied={applied} />
      </div>
    )
  }

  if (block.kind === 'table') {
    if (!block.rows?.length) {
      return (
        <Panel title={block.title}>
          <EmptyState title="No rows" description="Nothing matched the filters on this report." />
        </Panel>
      )
    }
    return (
      <div>
        <Panel title={block.title} flush
          meta={<span>{(block.total ?? block.rows.length).toLocaleString('en')} rows</span>}>
          <DataTable fields={fieldsOf(block)} rows={rowObjects(block)} height={420} />
        </Panel>
        <Unaffected block={block} view={view} applied={applied} />
      </div>
    )
  }

  return (
    <Panel title={block.title}>
      <p className="p-4 text-ink-secondary">{block.value}</p>
    </Panel>
  )
}

/**
 * A chart's bands, keeping the server's label exactly as written.
 *
 * Exported because this one line is the whole guarantee and it is worth a test.
 * The engine wrote the label — it knew whether the axis was a month or a
 * customer, and what to call it in either case. The portal's job is to draw it.
 *
 * It did not, for as long as the datum's key was called `month`: the axis ran
 * every label through a date formatter, so a report grouped by customer came
 * back as "c-1", "c-2", "c-3" and was drawn as January, February and March
 * 2001. JavaScript parses "c-1" as a date rather than refusing it, so there was
 * no error anywhere — just three customers wearing three months.
 */
export function bands(series: Bar[]): { label: string; value: number }[] {
  return series.map((s) => ({ label: s.label, value: s.value }))
}

/**
 * "Not affected by Region", on the block it does not affect.
 *
 * The server computes this — it is the only thing that can — and the report
 * format promises the interface will show it. A filter that quietly applies to
 * some blocks and not others is worse than one that admits it: someone reading
 * a filtered screen cannot tell which numbers moved, and will trust the ones
 * that did not.
 */
function Unaffected({ block, view, applied }: {
  block: ReportBlock
  view: ReportView
  applied: string[]
}) {
  // Only the ones somebody set. "Not affected by Region" above a screen nobody
  // has filtered is a fact about the report, and the reader has no question it
  // answers; the same words after they filter by Region are the difference
  // between trusting a number and misreading it.
  const ignored = (block.coverage?.ignored ?? []).filter((n) => applied.includes(n))
  if (ignored.length === 0) return null

  const labels = ignored.map((n) => view.filters?.find((f) => f.name === n)?.label ?? n)
  return (
    <p data-testid="unaffected" className="mt-2 text-caption text-ink-muted">
      Not affected by {labels.join(', ')}
    </p>
  )
}

/**
 * Table columns, from the labels the server sent.
 *
 * The portal's DataTable is driven by fields rather than by the row type, so
 * the server's column list becomes the field list directly. Alignment comes
 * over the wire because only the dataset knows which columns are measures.
 */
function fieldsOf(block: TableBlock): Field[] {
  return (block.columns ?? []).map((c, i) => ({
    name: `c${i}`,
    label: c.label,
    type: 'string',
    // The role carries alignment only: numbers that do not share a right edge
    // cannot be compared down a column. `preformatted` is what stops the table
    // formatting a value the engine already formatted.
    role: c.align === 'right' ? 'measure' : 'dimension',
    format: 'preformatted',
  })) as Field[]
}

/** Rows arrive as arrays; the table wants objects keyed to the fields. */
function rowObjects(block: TableBlock): Record<string, string>[] {
  return (block.rows ?? []).map((row) => {
    const out: Record<string, string> = {}
    row.forEach((cell, i) => {
      out[`c${i}`] = cell
    })
    return out
  })
}


