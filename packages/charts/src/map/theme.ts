import type { Tiles } from '../types'
import { el } from '../dom'
import { tileLayer, type TileLayer } from './tiles'

/**
 * Whether a map is drawn on a dark page: the theme an ancestor names in
 * `data-theme` — the portal's, or one a host page sets on the element —
 * looked for through every shadow root the map sits in, and otherwise the
 * reader's own system setting.
 */
export function darkPage(node: Element): boolean {
  let at: Element | null = node
  while (at) {
    const named = at.closest('[data-theme]')?.getAttribute('data-theme')
    if (named) return named === 'dark'
    const root = at.getRootNode()
    at = root instanceof ShadowRoot ? root.host : null
  }
  return globalThis.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false
}

/**
 * A basemap that follows the page: its dark tiles under a dark theme and its
 * light ones under a light one — asked again each time the map is laid out,
 * so a reader who switches theme gets the other tiles the next time the map
 * moves or settles. Where the tiles have one look it is the plain layer.
 * `changed` hears which tiles are down now, for the logo that goes with them.
 */
export function themedTiles(t: Tiles, stage: HTMLElement, changed: (now: Tiles) => void): TileLayer {
  const dark = t.dark
  if (!dark) return tileLayer(t)
  const element = el('div', { class: 'tiles-theme' })
  let showing: Tiles | null = null
  let inner: TileLayer | null = null
  return {
    element,
    zoom: () => inner?.zoom() ?? -1,
    lay(view, width, height) {
      const want = stage.isConnected && darkPage(stage) ? dark : t
      if (want !== showing || !inner) {
        showing = want
        inner = tileLayer(want)
        element.replaceChildren(inner.element)
        changed(want)
      }
      inner.lay(view, width, height)
    },
  }
}
