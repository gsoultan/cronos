/** Formatting used by tiles, axes and tables. One place, so nothing drifts. */

const COMPACT = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 })
const PLAIN = new Intl.NumberFormat('en')

/** Big standalone values: 1,284 · 12.9K · 4.2M */
export function compact(n: number): string {
  return Math.abs(n) < 10_000 ? PLAIN.format(Math.round(n)) : COMPACT.format(n)
}

export function currency(n: number, code = 'USD'): string {
  const f = new Intl.NumberFormat('en', {
    style: 'currency', currency: code,
    notation: Math.abs(n) >= 10_000 ? 'compact' : 'standard',
    maximumFractionDigits: Math.abs(n) >= 10_000 ? 1 : 2,
  })
  return f.format(n)
}

export function percent(n: number, digits = 1): string {
  return `${n > 0 ? '+' : ''}${n.toFixed(digits)}%`
}

/** Axis ticks round to clean numbers so they read at a glance. */
export function axisTick(n: number): string {
  return Math.abs(n) >= 1000 ? COMPACT.format(n) : PLAIN.format(n)
}

const DATE = new Intl.DateTimeFormat('en', { month: 'short', day: 'numeric' })
const MONTH = new Intl.DateTimeFormat('en', { month: 'short' })

export function shortDate(d: Date | string): string {
  const date = typeof d === 'string' ? new Date(d) : d
  if (Number.isNaN(date.getTime())) return String(d)
  return DATE.format(date)
}

/**
 * "Feb '26" — plain "Feb 26" reads as a day of the month on an axis.
 *
 * A value that is not a date comes back as it arrived. A chart's x axis is
 * whatever dimension it was grouped by, and grouping by customer is as
 * ordinary as grouping by month — formatting "Aurora Freight" as a date throws
 * from Intl, which takes the whole page down rather than one label with it.
 */
export function monthLabel(d: Date | string): string {
  const date = typeof d === 'string' ? new Date(d) : d
  if (Number.isNaN(date.getTime())) return String(d)
  return `${MONTH.format(date)} ’${String(date.getFullYear()).slice(2)}`
}

/** "3 minutes ago" — humans read elapsed time faster than timestamps. */
export function relativeTime(d: Date | string): string {
  const then = typeof d === 'string' ? new Date(d) : d
  const secs = Math.round((Date.now() - then.getTime()) / 1000)
  const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ['year', 31_536_000], ['month', 2_592_000], ['day', 86_400],
    ['hour', 3600], ['minute', 60],
  ]
  for (const [unit, size] of units) {
    if (Math.abs(secs) >= size) return rtf.format(-Math.round(secs / size), unit)
  }
  return 'just now'
}
