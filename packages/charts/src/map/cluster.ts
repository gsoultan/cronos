import type { Marker } from '../types'

/** Points close enough on screen to be drawn as one. */
export interface Cluster {
  /** Where to draw it: the middle of its members, in world units. */
  x: number
  y: number
  members: Marker[]
}

/**
 * Gathers markers closer than `cell` world units into clusters.
 *
 * Greedy over a grid rather than a hierarchy built once. A point joins the
 * first cluster whose seed is within reach in its own grid cell or a
 * neighbouring one, or seeds a new one: a pass per zoom level, each linear,
 * and five thousand points — the most a chart returns — is a few milliseconds.
 * A hierarchy precomputed at every zoom would be faster to consult and is a
 * library's worth of code in a bundle that has a budget.
 *
 * The seed rather than the moving centroid decides membership, so the same
 * points at the same zoom always cluster the same way whatever order a filter
 * returns them in; the centroid is only where the cluster is drawn.
 */
export function cluster(markers: Marker[], cell: number): Cluster[] {
  if (!(cell > 0)) return markers.map((m) => ({ x: m.x, y: m.y, members: [m] }))

  type Seed = { sx: number; sy: number; members: Marker[] }
  const grid = new Map<string, Seed[]>()
  const out: Seed[] = []

  // Placed by rank, so which point seeds a cluster does not depend on the
  // order a database happened to return rows in.
  const order = [...markers].sort((a, b) => a.x - b.x || a.y - b.y)
  for (const m of order) {
    const gx = Math.floor(m.x / cell)
    const gy = Math.floor(m.y / cell)
    const joined = nearest(grid, gx, gy, m, cell)
    if (joined) {
      joined.members.push(m)
      continue
    }
    const seed: Seed = { sx: m.x, sy: m.y, members: [m] }
    const key = `${gx},${gy}`
    const here = grid.get(key)
    if (here) here.push(seed)
    else grid.set(key, [seed])
    out.push(seed)
  }

  return out.map((s) => {
    // Weighted by the places each member stands for: a cell of a large map
    // holding forty pulls the circle towards it forty times as hard as a
    // cell holding one.
    let x = 0
    let y = 0
    let n = 0
    for (const m of s.members) {
      const w = m.n ?? 1
      x += m.x * w
      y += m.y * w
      n += w
    }
    return { x: x / n, y: y / n, members: s.members }
  })
}

/** How many places a cluster holds: its members, or the places they stand
 *  for when a member is a cell of a large map. */
export function placesIn(c: Cluster): number {
  let n = 0
  for (const m of c.members) n += m.n ?? 1
  return n
}

/** The closest seed within reach of m in its cell or the eight around it. */
function nearest(grid: Map<string, { sx: number; sy: number; members: Marker[] }[]>,
  gx: number, gy: number, m: Marker, cell: number) {
  let best: { sx: number; sy: number; members: Marker[] } | undefined
  let bestD = cell * cell
  for (let dx = -1; dx <= 1; dx++) {
    for (let dy = -1; dy <= 1; dy++) {
      for (const s of grid.get(`${gx + dx},${gy + dy}`) ?? []) {
        const d = (s.sx - m.x) ** 2 + (s.sy - m.y) ** 2
        if (d < bestD) {
          best = s
          bestD = d
        }
      }
    }
  }
  return best
}

/** A cluster's count, short enough for the circle it is written in. */
export function count(n: number): string {
  if (n < 1000) return String(n)
  if (n < 10_000) return `${Math.round(n / 100) / 10}k`
  return `${Math.round(n / 1000)}k`
}
