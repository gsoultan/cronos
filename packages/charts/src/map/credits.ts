import type { Tiles } from '../types'
import { el } from '../dom'
import { latitude, longitude } from './geo'
import type { View } from './view'

/** The line under a map naming whose basemap it is. */
export interface CreditLine {
  element: HTMLElement | null
  /** Tells the line what is in view, for a provider whose credit depends on
   *  it. */
  follow(view: View, zoom: number): void
}

/**
 * The credit line every tile source requires, in its own words and links, and
 * the reason when a basemap the author asked for is not there.
 *
 * Built from nodes, never markup: the parts come from the server, which wrote
 * them, but the rule in dom.ts is that nothing here parses HTML, and a credit
 * line is not the place to start. A link opens elsewhere and takes nothing
 * with it — no opener, no referrer.
 */
export function credits(tiles: Tiles | undefined, note: string | undefined): CreditLine {
  if (!tiles && !note) return { element: null, follow: () => {} }
  const line = el('p', { class: 'credit', part: 'attribution' })

  let first: HTMLElement | null = null
  if (tiles) {
    const parts = tiles.credits?.length ? tiles.credits : [{ text: tiles.attribution }]
    parts.forEach((c, i) => {
      const part = c.href
        ? el('a', { href: c.href, target: '_blank', rel: 'noopener noreferrer' }, c.text)
        : el('span', {}, c.text)
      if (i === 0) first = part
      line.append(...(i > 0 ? [' · ', part] : [part]))
    })
  }
  if (note) line.append(el('span', { class: 'note' }, note))

  return {
    element: line,
    follow: tiles?.viewport ? viewportCredit(tiles.viewport, () => first) : () => {},
  }
}

/**
 * Asks the provider for the credit line of what is in view.
 *
 * Google's copyright names whoever supplied the imagery on screen, so the
 * line the server sent is right for the map as it arrived and wrong once the
 * reader pans from a city to a desert. Asked for half a second after the view
 * stops moving, and only the latest answer kept: a drag produces a view a
 * frame, and one request per frame is a bill per frame.
 */
function viewportCredit(url: string, target: () => HTMLElement | null) {
  let timer: ReturnType<typeof setTimeout> | undefined
  let pending: AbortController | undefined
  let asked = ''

  return (view: View, zoom: number) => {
    clearTimeout(timer)
    timer = setTimeout(() => {
      const at = target()
      if (!at?.isConnected || zoom < 0) return
      const q = new URLSearchParams({
        zoom: String(zoom),
        north: latitude(view.y).toFixed(6), south: latitude(view.y + view.h).toFixed(6),
        west: longitude(view.x).toFixed(6), east: longitude(view.x + view.w).toFixed(6),
      }).toString()
      if (q === asked) return
      asked = q
      pending?.abort()
      pending = new AbortController()
      fetch(`${url}&${q}`, { signal: pending.signal, credentials: 'omit', referrerPolicy: 'strict-origin' })
        .then((r) => (r.ok ? r.json() : null))
        .then((body: { copyright?: unknown } | null) => {
          // Only ever text, and only ever replacing the provider's own part.
          if (body && typeof body.copyright === 'string' && body.copyright) at.textContent = body.copyright
        })
        // The line already there is right for somewhere near here. A failed
        // refresh keeps it rather than leaving the map with no credit at all.
        .catch(() => {})
    }, 500)
  }
}
