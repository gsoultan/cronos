import { useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { Field } from '../lib/types'
import { currency, shortDate } from '../lib/format'
import { StatusPill } from './StatusPill'

interface Props {
  fields: Field[]
  /** Any row shape — columns are driven by `fields`, not by the row type. */
  rows: readonly object[]
  /** Shown below the table so the count is never a mystery. */
  totalLabel?: string
  /** Scroll viewport height. Shorter inside a layout block than on a report. */
  height?: number
}

const ROW_H = 40

/**
 * A report can return a million rows; the DOM cannot. Only the visible window
 * is mounted, so scrolling stays smooth regardless of result size.
 */
export function DataTable({ fields, rows, totalLabel, height = 460 }: Props) {
  const parent = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parent.current,
    estimateSize: () => ROW_H,
    overscan: 12,
  })

  const cols = fields.filter((f) => !f.hidden)
  const template = cols.map((f) => (f.role === 'measure' ? '140px' : 'minmax(140px, 1fr)')).join(' ')

  return (
    /* The column template is fixed-width by design, so on a narrow screen the
       table scrolls sideways inside its own box. Letting it widen the document
       instead makes every other page element unreachable. */
    <div className="flex max-w-full flex-col overflow-x-auto">
      <div className="sticky top-0 z-2 grid border-b border-line bg-sunken"
        style={{ gridTemplateColumns: template }}>
        {cols.map((f) => (
          <div key={f.name}
            className={`px-4 py-2 text-caption font-semibold tracking-[0.04em]
                        text-ink-secondary uppercase ${f.role === 'measure' ? 'text-right' : ''}`}>
            {f.label}
          </div>
        ))}
      </div>

      <div ref={parent} className="overflow-auto [contain:strict]" style={{ height }}>
        <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
          {virtualizer.getVirtualItems().map((v) => {
            const row = rows[v.index] as Record<string, unknown>
            return (
              <div key={v.key}
                className="absolute top-0 left-0 grid w-full items-center border-b
                           border-line hover:bg-hover"
                style={{
                  gridTemplateColumns: template,
                  transform: `translateY(${v.start}px)`,
                  height: ROW_H,
                }}>
                {cols.map((f) => (
                  <div key={f.name}
                    className={`truncate px-4 text-small ${
                      f.role === 'measure' ? 'text-right tabular-nums' : ''}`}>
                    {renderCell(f, row[f.name])}
                  </div>
                ))}
              </div>
            )
          })}
        </div>
      </div>

      {totalLabel && (
        <div className="border-t border-line bg-sunken px-4 py-3 text-small text-ink-secondary">
          {totalLabel}
        </div>
      )}
    </div>
  )
}

function renderCell(field: Field, value: unknown) {
  if (value === null || value === undefined || value === '') {
    return <span className="text-ink-muted">—</span>
  }
  if (field.name === 'status') return <StatusPill value={String(value)} />
  if (field.type === 'date') return shortDate(String(value))
  if (field.format === 'currency') return currency(Number(value))
  if (field.role === 'measure') return Number(value).toLocaleString('en')
  return String(value)
}
