import { Client } from './client'
import { css } from './styles'
import { el, fill } from './dom'
import { unaffectedNote } from './coverage'
import { statBlock } from './blocks/stat'
import { barBlock } from './blocks/bar'
import { tableBlock } from './blocks/table'
import type { Block, FilterDef, FilterValues, ReportPayload } from './types'

/*
 * Nothing here runs at import time.
 *
 * A React host is usually a React *framework* host — Next.js, Remix — and the
 * module graph is evaluated on a server where there is no HTMLElement and no
 * CSSStyleSheet. `class X extends undefined` throws while the page is being
 * rendered, so importing this package would break a route that had not even
 * mounted the component yet. The stand-in below is never instantiated;
 * registration is guarded separately.
 */
const Base: typeof HTMLElement =
  typeof HTMLElement === 'undefined'
    ? (class {} as unknown as typeof HTMLElement)
    : HTMLElement

/** Constructed once and adopted by every instance, rather than parsed per element. */
let sheet: CSSStyleSheet | undefined
function styles(): CSSStyleSheet {
  if (!sheet) {
    sheet = new CSSStyleSheet()
    sheet.replaceSync(css)
  }
  return sheet
}

/**
 * `<cronos-report endpoint token report>`.
 *
 * A custom element rather than an iframe. An iframe is the easy isolation, but
 * it cannot size itself to its content, it cannot inherit a host's theme, and
 * it makes every report a scrollbar inside a page. A shadow root gives the
 * same style isolation with none of that.
 */
export class CronosReport extends Base {
  static observedAttributes = ['endpoint', 'token', 'report']

  #root = this.attachShadow({ mode: 'open' })
  #body = el('div', { class: 'grid', part: 'grid' })
  #filters: FilterValues = {}
  #filterKey = '{}'
  #inflight: AbortController | null = null
  /** Connected is tracked so an attribute set before insertion does not fetch. */
  #live = false

  connectedCallback() {
    this.#root.adoptedStyleSheets = [styles()]
    fill(this.#root, this.#body)
    this.#live = true
    void this.load()
  }

  disconnectedCallback() {
    this.#live = false
    // A report removed mid-request must not resolve into a detached tree, and
    // must not hold the connection open on a page that has moved on.
    this.#inflight?.abort()
  }

  attributeChangedCallback() {
    if (this.#live) void this.load()
  }

  /**
   * The filters to apply. Assigning reloads.
   *
   * A property and not an attribute: filters are structured, and serialising
   * them through an attribute would invite a host page to build that string
   * themselves.
   */
  get filters(): FilterValues {
    return this.#filters
  }

  set filters(next: FilterValues) {
    /* Compared by value, not identity. Every framework re-creates an inline
       object on each render — `filters={{ status: … }}` in React,
       `:filters="{ … }"` in Vue — so an identity check here would refetch on
       every keystroke elsewhere on the host's page, forever. Use load() to
       force a refresh. */
    const key = JSON.stringify(next ?? {})
    if (key === this.#filterKey) return
    this.#filterKey = key
    this.#filters = next ?? {}
    if (this.#live) void this.load()
  }

  async load(): Promise<void> {
    const endpoint = this.getAttribute('endpoint')
    const token = this.getAttribute('token')
    const report = this.getAttribute('report')
    if (!endpoint || !token || !report) {
      return this.#say('Set endpoint, token and report.', true)
    }

    // One request at a time. Filters change faster than a report loads, and a
    // slow earlier response landing after a fast later one shows the wrong
    // numbers with nothing on screen saying so.
    this.#inflight?.abort()
    const run = new AbortController()
    this.#inflight = run

    this.#say('Loading…', false)
    try {
      const payload = await new Client(endpoint, token).report(report, this.#filters, run.signal)
      if (run.signal.aborted) return
      this.#render(payload)
      this.dispatchEvent(new CustomEvent('cronos:load', { detail: { report } }))
    } catch (err) {
      if (run.signal.aborted) return
      const text = err instanceof Error ? err.message : 'The report could not be loaded.'
      this.#say(text, true)
      this.dispatchEvent(new CustomEvent('cronos:error', { detail: { message: text } }))
    }
  }

  #render(payload: ReportPayload) {
    const filters = payload.filters ?? []
    fill(this.#body, ...payload.blocks.map((b) => this.#block(b, filters)))
  }

  #block(b: Block, filters: FilterDef[]): HTMLElement {
    const node =
      b.kind === 'stat' ? statBlock(b) : b.kind === 'bar' ? barBlock(b) : tableBlock(b)
    const note = unaffectedNote(b.coverage, filters)
    if (note) node.append(note)
    return node
  }

  #say(text: string, isError: boolean) {
    fill(this.#body, el('p', { class: isError ? 'msg err' : 'msg', part: 'message' }, text))
  }
}
