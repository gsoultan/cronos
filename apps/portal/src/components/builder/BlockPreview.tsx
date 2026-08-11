import { useMemo } from 'react'
import { StatTile } from '../StatTile'
import { ColumnChart } from '../ColumnChart'
import { LineChart } from '../LineChart'
import { DataTable } from '../DataTable'
import { sampleRows } from '../../lib/sampleRows'
import { currency } from '../../lib/format'
import type { Field, Tile } from '../../lib/types'

/**
 * The block, drawn with the components that will actually render it.
 *
 * Earlier this was a sketch — coloured rectangles standing in for charts. That
 * is fine beside a config form, and useless as the main view: the whole point
 * of a canvas is that the thing on screen is the thing you are shipping. These
 * are the same `StatTile`, `ColumnChart`, `LineChart` and `DataTable` the
 * report renders, fed sample rows.
 */
export function BlockPreview({ block, fields }: { block: Tile; fields: Field[] }) {
  const field = fields.find((f) => f.name === block.field)
  const group = fields.find((f) => f.name === block.groupBy)

  /* Vary the sample by block, or two tiles side by side show the same number
     and read as a rendering bug rather than as placeholder data. */
  const jitter = useMemo(() => seedOf(block.id), [block.id])
  const series = useMemo(
    () => months().map((m, i) => ({
      month: m,
      value: Math.round((240_000 + jitter * 90_000) + i * (18_000 + jitter * 9_000) + (i % 2) * 14_000),
    })),
    [jitter],
  )
  const columns = useMemo(
    () => (block.columns ?? []).map((c) => fields.find((f) => f.name === c)).filter(Boolean) as Field[],
    [block.columns, fields],
  )
  const rows = useMemo(() => sampleRows(columns, 40), [columns])

  switch (block.kind) {
    case 'stat':
      return (
        <StatTile label={field?.label ?? block.title}
          value={currency(Math.round(12_000 + jitter * 86_000))}
          delta={Math.round((jitter * 18 - 6) * 10) / 10} deltaPeriod="last month" />
      )

    case 'bar':
      return (
        <ColumnChart title={block.title}
          subtitle={group ? `By ${group.label.toLowerCase()}` : undefined}
          data={series.map((s) => ({ month: s.month, Value: s.value }))}
          series={['Value']} />
      )

    case 'line':
      return (
        <LineChart title={block.title}
          subtitle={group ? `By ${group.label.toLowerCase()}` : undefined}
          data={series.map((s) => ({ month: s.month, value: Math.round(s.value / 4000) }))}
          format={(n) => n.toLocaleString('en')} />
      )

    case 'table':
      return (
        <div className="overflow-hidden rounded-lg border border-line bg-surface shadow-card">
          <div className="border-b border-line px-4 py-3">
            <h3 className="text-lead font-semibold text-ink">{block.title}</h3>
          </div>
          {columns.length === 0 ? (
            <p className="px-4 py-10 text-center text-small text-ink-muted">
              No columns chosen yet.
            </p>
          ) : (
            <DataTable fields={columns} rows={rows} height={220} />
          )}
        </div>
      )

    default:
      return null
  }
}

/** Stable per block id, so a preview does not reshuffle as you edit. */
function seedOf(id: string): number {
  let h = 0
  for (const ch of id) h = (h * 31 + ch.charCodeAt(0)) % 1000
  return h / 1000
}

function months(): string[] {
  return ['2026-02-01', '2026-03-01', '2026-04-01', '2026-05-01', '2026-06-01', '2026-07-01']
}
