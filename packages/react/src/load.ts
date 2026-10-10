/**
 * Loads `<cronos-report>` from the cronos server the report comes from.
 *
 * Not bundled into this package: served by cronosd, the element is always the
 * build of the API it calls. Bundled, it is whatever version the host last
 * installed, and an upgraded server meets an old element as an empty panel.
 *
 * `endpoint` now decides which code runs on the host's page, not only where
 * its data comes from. So it must be an absolute http(s) URL — `data:` and
 * `blob:` would import whatever they contain — and it must be a constant in
 * the host's own code, never read from a query string or anything a visitor
 * sets. The host's CSP `script-src` is the control that holds even if it is.
 */
export const PATH = '/v1/embed/cronos-embed.js'

const TAG = 'cronos-report'

/* One import per page. The element can be defined once, so a second endpoint
   on the same page would load a script whose definition is refused — the
   first one wins, and they are the same element. */
let pending: Promise<void> | undefined

export function load(endpoint: string): Promise<void> {
  if (typeof customElements === 'undefined') return Promise.resolve()
  if (customElements.get(TAG)) return Promise.resolve()
  if (!pending) {
    pending = importFrom(endpoint).catch((err: unknown) => {
      // Forgotten on failure, so the next mount tries again rather than
      // inheriting a network blip for the life of the page.
      pending = undefined
      throw err
    })
  }
  return pending
}

/** The URL the element is loaded from, or an error saying why there is none. */
export function scriptURL(endpoint: string): string {
  let url: URL
  try {
    url = new URL(endpoint)
  } catch {
    throw new Error(`cronos: endpoint must be an absolute URL, not ${JSON.stringify(endpoint)}`)
  }
  if (url.protocol !== 'https:' && url.protocol !== 'http:') {
    throw new Error(`cronos: endpoint must be http(s), not ${url.protocol}`)
  }
  // Joined as the element joins its own API calls, so a cronos behind a path
  // prefix serves both from the same place.
  return endpoint.replace(/\/$/, '') + PATH
}

async function importFrom(endpoint: string): Promise<void> {
  await import(/* @vite-ignore */ scriptURL(endpoint))
  await customElements.whenDefined(TAG)
}
