import type { GeoMap, MapDetail, MapViewAsk, MapViewer } from '../types'
import type { View } from './view'

/** How long the map waits after a gesture settles before asking, so a reader
 *  clicking + three times asks once. */
const WAIT = 160

/** Keeps a large map's cells in step with where its reader is looking. */
export interface Refiner {
  /** The view has settled here; ask for it, soon. */
  follow(view: View): void
  /** The map has left the page; drop whatever is on its way. */
  stop(): void
}

/**
 * A large map asks the server for the part in view each time a reader stops
 * moving it — at the depth they are looking at, in cells a few pixels wide, so
 * a zoom from the country to a street goes from a density of a million places
 * to the places themselves.
 *
 * One question at a time, and only the latest answer drawn: a reader zooms
 * faster than a warehouse answers, and an earlier view arriving after a later
 * one would paint the wrong street under the right one. A question already
 * answered is not asked again.
 */
export function refiner(detail: MapDetail, viewer: MapViewer, stage: HTMLElement,
  apply: (m: GeoMap) => void): Refiner {

  let timer: ReturnType<typeof setTimeout> | undefined
  let inflight: AbortController | null = null
  let asked = ''
  let seq = 0

  const ask = async (view: View) => {
    const width = Math.round(stage.clientWidth)
    const height = Math.round(stage.clientHeight)
    if (!stage.isConnected || width <= 0 || height <= 0) return
    const q = question(detail, view, width, height)
    const key = JSON.stringify(q)
    if (key === asked) return
    asked = key
    inflight?.abort()
    const ctl = new AbortController()
    inflight = ctl
    const mine = ++seq
    stage.classList.add('busy')
    try {
      const got = await viewer(q, ctl.signal)
      if (mine === seq && !ctl.signal.aborted) apply(got)
    } catch {
      // What is drawn stays drawn, and the next settle asks again: a view
      // that failed is not an answer to remember.
      if (mine === seq) asked = ''
    } finally {
      if (mine === seq) stage.classList.remove('busy')
    }
  }

  return {
    follow(view) {
      clearTimeout(timer)
      timer = setTimeout(() => void ask(view), WAIT)
    },
    stop() {
      clearTimeout(timer)
      inflight?.abort()
      seq++
    },
  }
}

/** The question for a view: rounded, so the same view is the same question. */
export function question(d: MapDetail, v: View, width: number, height: number): MapViewAsk {
  const r = (n: number) => Math.round(n * 1e9) / 1e9
  const q: MapViewAsk = {
    output: d.output, block: d.block,
    view: [r(v.x), r(v.y), r(v.x + v.w), r(v.y + v.h)], width, height,
  }
  if (d.overlay) q.overlay = d.overlay
  if (d.categories?.length) q.categories = d.categories
  return q
}
