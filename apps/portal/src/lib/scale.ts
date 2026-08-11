/** Axis ticks land on clean numbers (0 / 1,000 / 2,000) so they read at a glance. */
export function niceTicks(max: number, count: number): number[] {
  if (!Number.isFinite(max) || max <= 0) return [0, 1]
  const raw = max / count
  const mag = 10 ** Math.floor(Math.log10(raw))
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw) ?? mag * 10
  const out: number[] = []
  for (let v = 0; v <= max + step * 0.001; v += step) out.push(Number(v.toFixed(6)))
  return out
}

/** Same, but for a band that need not start at zero (rates, percentages). */
export function niceBand(min: number, max: number, count: number): number[] {
  if (min === max) return [min]
  const raw = (max - min) / count
  const mag = 10 ** Math.floor(Math.log10(raw))
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw) ?? mag * 10
  const lo = Math.floor(min / step) * step
  const hi = Math.ceil(max / step) * step
  const out: number[] = []
  for (let v = lo; v <= hi + step * 0.001; v += step) out.push(Number(v.toFixed(6)))
  return out
}
