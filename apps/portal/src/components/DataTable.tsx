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
/* Narrower than this and a column label wraps, which costs more height than
   the horizontal scroll it saves. Shared with the template below so the one
   scroller's minimum and the tracks it is scrolling cannot disagree. */
const MIN_COL = 140

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
  const template = cols
    .map((f) => (f.role === 'measure' ? `${MIN_COL}px` : `minmax(${MIN_COL}px, 1fr)`))
    .join(' ')

  const viewport = windowHeight(rows.length, height)

  return (
    /* One horizontal scroller, around both halves. The column template is
       fixed-width by design — letting it widen the document makes every other
       element on the page unreachable — but the rows used to carry a scroller
       of their own, so dragging them sideways left the headings behind and
       every column was then labelled with its neighbour's name. The minimum
       width is what gives this box something to scroll; inside it neither half
       overflows, so neither can move without the other. */
    <div className="max-w-full overflow-x-auto">
      <div style={{ minWidth: cols.length * MIN_COL }}>
        <div data-testid="table-head" className="grid border-b border-line bg-sunken"
          style={{ gridTemplateColumns: template }}>
          {cols.map((f) => (
            <div key={f.name}
              className={`px-4 py-2 text-caption font-semibold tracking-[0.04em]
                          text-ink-secondary uppercase ${f.role === 'measure' ? 'text-right' : ''}`}>
              {f.label}
            </div>
          ))}
        </div>

        <div ref={parent} data-testid="table-rows"
          className="overflow-x-clip overflow-y-auto [contain:strict]"
          style={{ height: viewport }}>
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
    </div>
  )
}

/**
 * How tall a table's window is: as tall as its rows, up to the cap.
 *
 * The cap used to be the height, whatever the result was, so a three-row table
 * on a report opened a three-hundred-pixel hole under itself — which reads as a
 * load that failed rather than as a short answer. The cap is what keeps a
 * million-row result virtualised, and it only applies once there are rows
 * enough to reach it. One row's height at the least, so an empty window is
 * still visibly a table.
 */
export function windowHeight(rows: number, cap: number): number {
  return Math.min(Math.max(rows, 1) * ROW_H, cap)
}

function renderCell(field: Field, value: unknown) {
  if (value === null || value === undefined || value === '') {
    return <span className="text-ink-muted">—</span>
  }
  /* Formatted already, by the engine that knew the currency and the locale.
     Reformatting a value that arrived as "19,800" means Number("19,800"),
     which is NaN — the column reads as broken rather than as data. */
  if (field.format === 'preformatted') return String(value)
  if (field.name === 'status') return <StatusPill value={String(value)} />
  if (field.type === 'date') return shortDate(String(value))
  if (field.format === 'currency') return currency(Number(value))
  if (field.role === 'measure') return Number(value).toLocaleString('en')
  return String(value)
}
