import { StatTile } from './StatTile'
import { ColumnChart } from './ColumnChart'
import { DataTable } from './DataTable'
import { Panel } from './Panel'
import { EmptyState } from './EmptyState'
import type { ReportBlock, ReportView } from '../lib/api'
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
export function LiveReport({ view }: { view: ReportView }) {
  const stats = view.blocks.filter((b) => b.kind === 'stat')
  const rest = view.blocks.filter((b) => b.kind !== 'stat')

  return (
    <div className="grid gap-6" data-testid="live-report">
      {stats.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {stats.map((b, i) => (
            <div key={b.title + i}>
              <StatTile label={b.title} value={b.value ?? '—'} hero={i === 0} />
              <Unaffected block={b} view={view} />
            </div>
          ))}
        </div>
      )}
      {rest.map((b, i) => (
        <Block key={b.title + i} block={b} view={view} />
      ))}
    </div>
  )
}

function Block({ block, view }: { block: ReportBlock; view: ReportView }) {
  if (block.kind === 'chart') {
    if (block.chart !== 'bar' || !block.series?.length) {
      // A chart type this build does not draw is a normal condition once the
      // server ships one first. Saying so beats an empty panel.
      return (
        <Panel title={block.title}>
          <p className="p-6 text-center text-small text-ink-muted">
            {block.series?.length ? `${block.chart} charts need a newer portal` : 'No data'}
          </p>
        </Panel>
      )
    }
    return (
      <div>
        <ColumnChart
          title={block.title}
          data={block.series.map((s) => ({ month: s.label, value: s.value }))}
          series={['value']}
          format={(n) => formatted(block, n)}
        />
        <Unaffected block={block} view={view} />
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
        <Unaffected block={block} view={view} />
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
 * "Not affected by Region", on the block it does not affect.
 *
 * The server computes this — it is the only thing that can — and the report
 * format promises the interface will show it. A filter that quietly applies to
 * some blocks and not others is worse than one that admits it: someone reading
 * a filtered screen cannot tell which numbers moved, and will trust the ones
 * that did not.
 */
function Unaffected({ block, view }: { block: ReportBlock; view: ReportView }) {
  const ignored = block.coverage?.ignored ?? []
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
function fieldsOf(block: ReportBlock): Field[] {
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
function rowObjects(block: ReportBlock): Record<string, string>[] {
  return (block.rows ?? []).map((row) => {
    const out: Record<string, string> = {}
    row.forEach((cell, i) => {
      out[`c${i}`] = cell
    })
    return out
  })
}

/**
 * The chart's own formatted label for a value, falling back to the number.
 *
 * The axis and the tooltip should say what the engine said. Reformatting here
 * would mean a bar labelled one way and the table below it another.
 */
function formatted(block: ReportBlock, n: number): string {
  const match = block.series?.find((s) => s.value === n)
  return match?.formatted ?? n.toLocaleString('en')
}
