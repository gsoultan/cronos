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
  /** What a column draws in its cells, by field name: a data bar per row
   *  behind the figure, or a sparkline per row in place of one. The server
   *  worked both out; the table only places them. */
  marks?: Record<string, ColumnMarks>
}

export interface ColumnMarks {
  /** -1 to 1 of the column's largest. */
  bars?: number[]
  /** Points 0 to 1 up the cell, each row against its own range. */
  spark?: number[][]
  /** Each row's step on the heatmap ramp, from 0, or -1 for a plain cell. */
  shades?: number[]
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
export function DataTable({ fields, rows, totalLabel, height = 460, marks }: Props) {
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
                  {cols.map((f) => {
                    const bar = marks?.[f.name]?.bars?.[v.index]
                    const line = marks?.[f.name]?.spark?.[v.index]
                    const shade = marks?.[f.name]?.shades?.[v.index] ?? -1
                    return (
                      <div key={f.name} style={shade >= 0 ? shadeOf(shade) : undefined}
                        className={`relative truncate px-4 text-small ${bar || line || shade >= 0 ? 'flex h-full items-center' : ''} ${
                          f.role === 'measure' ? 'justify-end text-right tabular-nums' : ''}`}>
                        {bar ? <DataBar share={bar} /> : null}
                        {line && line.length > 1
                          ? <RowSpark points={line} />
                          // The figure is lifted over its bar; a cell with
                          // none keeps its text as the cell's own.
                          : bar ? <span className="relative truncate">{renderCell(f, row[f.name])}</span>
                            : renderCell(f, row[f.name])}
                      </div>
                    )
                  })}
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

export function renderCell(field: Field, value: unknown) {
  /* Except an empty string the engine wrote. It writes a null as a dash
     itself, so its empty cell is a different answer: a pivot's pair that no
     row reached, which is blank so a dash can still mean "matched, and null". */
  if (value === null || value === undefined || (value === '' && field.format !== 'preformatted')) {
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

/**
 * A data bar: behind the figure, from the left of the cell, as long as its
 * share of the column's largest. Red below nothing, so a loss is not read as
 * a short gain.
 */
function DataBar({ share }: { share: number }) {
  return (
    <span data-testid="databar" aria-hidden
      className={`absolute inset-y-2.5 left-3 rounded-sm ${
        share < 0 ? 'bg-[color-mix(in_srgb,var(--color-delta-bad)_18%,transparent)]'
          : 'bg-[color-mix(in_srgb,var(--color-accent)_18%,transparent)]'}`}
      style={{ width: `calc((100% - 1.75rem) * ${Math.abs(share)})` }} />
  )
}

const SPARK_W = 96
const SPARK_H = 22

/** A row's line, its last point in the accent. */
function RowSpark({ points }: { points: number[] }) {
  const x = (i: number) => 3 + (i / (points.length - 1)) * (SPARK_W - 6)
  const y = (v: number) => 3 + (1 - v) * (SPARK_H - 6)
  const d = points.map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ')
  const last = points.length - 1
  return (
    <svg data-testid="rowspark" width={SPARK_W} height={SPARK_H} aria-hidden className="block">
      <path d={d} fill="none" stroke="var(--color-series-muted)" strokeWidth={1.5}
        strokeLinecap="round" strokeLinejoin="round" />
      <circle cx={x(last)} cy={y(points[last] ?? 0.5)} r={2.5} fill="var(--color-accent)" />
    </svg>
  )
}

/**
 * A shaded pivot cell: the accent, stronger the further up the ramp, over
 * whichever surface the theme has. The figure stays in ink: the strongest
 * step stops short of where ink would stop reading on it in either theme.
 */
export function shadeOf(step: number): { background: string } {
  const strength = 12 + Math.min(step, 5) * 13
  return { background: `color-mix(in srgb, var(--color-accent) ${strength}%, transparent)` }
}
