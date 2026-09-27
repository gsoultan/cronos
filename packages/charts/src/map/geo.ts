import type { Bounds } from '../types'
import type { View } from './view'

/**
 * A world-unit number as SVG geometry.
 *
 * Not `n` from svg.ts, which rounds to three decimals — a sub-pixel in a
 * chart's user units, and forty kilometres in world units, where the whole
 * planet is 1.0. Every dot on a map went through it: on a country-sized map
 * each one jumped by up to a few percent of the width, and on a city-sized
 * one every radius was worked out as a fraction of a viewBox a few thousandths
 * across and rounded to nothing, so the dots, the heat field and the flows
 * were not drawn at all. Nine decimals is four millimetres, which is under a
 * pixel at the deepest zoom any tile source serves.
 */
export function g(v: number): string {
  return Number.isFinite(v) ? String(Math.round(v * 1e9) / 1e9) : '0'
}

/** A view as a viewBox attribute. */
export function viewBox(v: View): string {
  return `${g(v.x)} ${g(v.y)} ${g(v.w)} ${g(v.h)}`
}

/** The latitude at a world-unit y — the projection's inverse, for the one
 *  request that has to speak degrees. */
export function latitude(y: number): number {
  return (Math.atan(Math.sinh(Math.PI * (1 - 2 * y))) * 180) / Math.PI
}

/** The longitude at a world-unit x. */
export function longitude(x: number): number {
  return x * 360 - 180
}

/** The box around a set of points, grown by a margin so none sits on the
 *  edge of the view that shows them. */
export function around(points: { x: number; y: number }[], margin = 0.2): Bounds {
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (const p of points) {
    minX = Math.min(minX, p.x)
    minY = Math.min(minY, p.y)
    maxX = Math.max(maxX, p.x)
    maxY = Math.max(maxY, p.y)
  }
  const pad = Math.max(maxX - minX, maxY - minY) * margin
  return { minX: minX - pad, minY: minY - pad, maxX: maxX + pad, maxY: maxY + pad }
}
