import type { Field } from './types'

const WORDS = [
  'Alpine Freight', 'Bergen Logistics', 'Cobalt Manufacturing', 'Delta Foods',
  'Eastward Trading', 'Fjord Systems', 'Granite Supply', 'Harbour Retail',
]
const STATUSES = ['Paid', 'Sent', 'Overdue', 'Draft']
const REGIONS = ['North', 'South', 'East', 'West']

/** Deterministic, so a preview does not reshuffle on every keystroke. */
function rand(seed: number): number {
  const x = Math.sin(seed * 12.9898) * 43758.5453
  return x - Math.floor(x)
}

/**
 * Plausible rows for whatever columns the query currently returns.
 *
 * Reusing a fixed fixture meant the preview showed em-dashes for any column the
 * fixture happened not to have, which reads as "the query is broken" rather
 * than "this is a sketch". Generating from the field list means the preview is
 * always populated and always matches the query being built.
 */
export function sampleRows(fields: Field[], count = 50): Record<string, unknown>[] {
  return Array.from({ length: count }, (_, i) =>
    Object.fromEntries(fields.map((f, j) => [f.name, valueFor(f, i * 31 + j * 7 + 1)])),
  )
}

function valueFor(f: Field, seed: number): unknown {
  const r = rand(seed)

  if (/status/i.test(f.name)) return STATUSES[Math.floor(r * STATUSES.length)]
  if (/region/i.test(f.name)) return REGIONS[Math.floor(r * REGIONS.length)]
  if (/email/i.test(f.name)) {
    return `${WORDS[Math.floor(r * WORDS.length)]!.split(' ')[0]!.toLowerCase()}@example.com`
  }
  if (/(^|_)id$/i.test(f.name)) return `${f.name.slice(0, 3).toUpperCase()}-${10_000 + seed}`

  switch (f.type) {
    case 'date': {
      const d = new Date(2026, 1, 1)
      d.setDate(d.getDate() + Math.floor(r * 180))
      return d.toISOString()
    }
    case 'number':
      return Math.round(r * 5_000)
    case 'decimal':
      return Math.round((400 + r * 24_000) * 100) / 100
    case 'bool':
      return r > 0.5
    default:
      return WORDS[Math.floor(r * WORDS.length)]
  }
}
