/**
 * A number as the server's compact() writes one: grouped below a million,
 * then M and B. The server formats every value it sends; this is for the few a
 * viewer works out itself — a cluster's total, a cell's.
 */
export function compact(v: number): string {
  const abs = Math.abs(v)
  if (abs >= 1e9) return `${trim(v / 1e9)}B`
  if (abs >= 1e6) return `${trim(v / 1e6)}M`
  return grouped(v)
}

/** Thousands separated, whole numbers kept whole and the rest to two
 *  places — the server's group and decimalsFor, so a figure a viewer works out
 *  reads like the ones it was sent. */
export function grouped(v: number): string {
  const [int = '', frac] = Math.abs(v).toFixed(Number.isInteger(v) ? 0 : 2).split('.')
  return (v < 0 ? '-' : '') + int.replace(/\B(?=(\d{3})+(?!\d))/g, ',') + (frac ? `.${frac}` : '')
}

function trim(v: number): string {
  return v.toFixed(1).replace(/\.0$/, '')
}
