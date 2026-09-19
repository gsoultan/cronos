import { Client } from './client'
import { css } from './styles'
import { el, fill } from './dom'
import { unaffectedNote } from './coverage'
import { filterBar } from './filters'
import { statBlock } from './blocks/stat'
import { barBlock } from './blocks/bar'
import { lineBlock } from './blocks/line'
import { pieBlock } from './blocks/pie'
import { scatterBlock } from './blocks/scatter'
import { mapBlock } from './blocks/map'
import { comboBlock } from './blocks/combo'
import { funnelBlock } from './blocks/funnel'
import { waterfallBlock } from './blocks/waterfall'
import { heatmapBlock } from './blocks/heatmap'
import { gaugeBlock } from './blocks/gauge'
import { treemapBlock } from './blocks/treemap'
import { tableBlock } from './blocks/table'
import { unsupported } from './blocks/unsupported'
import type { Block, ChartBlock, FilterDef, FilterValues, ReportPayload } from './types'

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
  static observedAttributes = ['endpoint', 'token', 'report', 'controls']

  #root = this.attachShadow({ mode: 'open' })
  #bar = el('div', { class: 'bar' })
  #body = el('div', { class: 'grid', part: 'grid' })
  #filters: FilterValues = {}
  #filterKey = '{}'
  #inflight: AbortController | null = null
  /** Connected is tracked so an attribute set before insertion does not fetch. */
  #live = false

  connectedCallback() {
    this.#root.adoptedStyleSheets = [styles()]
    fill(this.#root, this.#bar, this.#body)
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
    /* The report format has always described these as controls on a filter
       bar; nothing drew one, so every host page built its own. It is drawn
       here now, and `controls="none"` is how a host that already has its own
       keeps driving `.filters` without getting a second set. */
    const bar = this.getAttribute('controls') === 'none'
      ? null
      : filterBar(filters, this.#filters, (next) => { this.filters = next })
    fill(this.#bar, ...(bar ? [bar] : []))
    fill(this.#body, ...payload.blocks.map((b) => this.#block(b, filters)))
  }

  /* An explicit switch with a default, not a chain of ternaries. The previous
     version fell through to the table renderer for anything it did not
     recognise, which meant a block kind it had never heard of crashed on
     `columns.map` — a server that grows a feature must not take a host page
     down with it. */
  #block(b: Block, filters: FilterDef[]): HTMLElement {
    const node = this.#draw(b)
    const note = unaffectedNote(b.coverage, filters)
    if (note) node.append(note)
    return node
  }

  #draw(b: Block): HTMLElement {
    switch (b.kind) {
      case 'stat':
        return statBlock(b)
      case 'chart':
        return this.#chart(b)
      case 'table':
        return tableBlock(b)
      default:
        return unsupported('This block needs a newer viewer')
    }
  }

  /* A second explicit switch, for the same reason as the first. The chart type
     is an open set the server grows — a type this build has never heard of is
     a normal condition, not a bug, and saying so beats an empty panel. */
  #chart(b: ChartBlock): HTMLElement {
    switch (b.chart) {
      case 'bar':
        return barBlock(b)
      case 'line':
        return lineBlock(b, false)
      case 'area':
        return lineBlock(b, true)
      case 'pie':
        return pieBlock(b, false)
      case 'donut':
        return pieBlock(b, true)
      case 'scatter':
      case 'bubble':
        return scatterBlock(b)
      case 'map':
        return mapBlock(b)
      case 'combo':
        return comboBlock(b)
      case 'funnel':
        return funnelBlock(b)
      case 'waterfall':
        return waterfallBlock(b)
      case 'heatmap':
        return heatmapBlock(b)
      case 'gauge':
        return gaugeBlock(b)
      case 'treemap':
        return treemapBlock(b)
      default:
        return unsupported(`${b.chart} charts need a newer viewer`)
    }
  }

  /**
   * A message in place of the blocks — never in place of the bar.
   *
   * Clearing the controls when a request fails is how a filter that matched
   * nothing becomes unrecoverable: the reader is left with an error and no
   * way to undo what caused it.
   */
  #say(text: string, isError: boolean) {
    fill(this.#body, el('p', { class: isError ? 'msg err' : 'msg', part: 'message' }, text))
  }
}
