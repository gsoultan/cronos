import { beforeEach, describe, expect, test } from 'bun:test'

/*
 * A document just big enough for the tile layer: elements that hold children,
 * attributes, a style, a dataset and listeners — and images that load when the
 * test says so, which is the part a browser will not let a test decide.
 */
class FakeElement {
  children: FakeElement[] = []
  parent: FakeElement | null = null
  attributes = new Map<string, string>()
  style: Record<string, string> = {}
  dataset: Record<string, string> = {}
  listeners = new Map<string, (() => void)[]>()
  constructor(readonly tagName: string) {}
  setAttribute(k: string, v: string) { this.attributes.set(k, v) }
  getAttribute(k: string) { return this.attributes.get(k) ?? null }
  addEventListener(type: string, fn: () => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn])
  }
  append(...nodes: FakeElement[]) {
    for (const n of nodes) {
      n.remove()
      n.parent = this
      this.children.push(n)
    }
  }
  remove() {
    if (!this.parent) return
    this.parent.children = this.parent.children.filter((c) => c !== this)
    this.parent = null
  }
  fire(type: string) { for (const fn of this.listeners.get(type) ?? []) fn() }
}

let images: FakeElement[] = []
beforeEach(() => {
  images = []
  ;(globalThis as { document?: unknown }).document = {
    createElement(tag: string) {
      const e = new FakeElement(tag)
      if (tag === 'img') images.push(e)
      return e
    },
  }
})

const { tileLayer } = await import('../src/map/tiles')
const tiles = { url: '{z}/{x}/{y}.png', attribution: '', maxZoom: 19, tileSize: 256 }
const zoomOf = (img: FakeElement) => Number(img.getAttribute('src')!.split('/')[0])

describe('replacing a zoom level', () => {
  test('keeps the old level until every tile of the new one has arrived', () => {
    const layer = tileLayer(tiles)
    const root = layer.element as unknown as FakeElement

    // The whole world at zoom 0: one tile, arrived.
    layer.lay({ x: 0, y: 0, w: 1, h: 1 }, 256, 256)
    images.forEach((i) => i.fire('load'))

    // In one level: four tiles asked for, none arrived, the old level kept.
    layer.lay({ x: 0, y: 0, w: 0.5, h: 0.5 }, 256, 256)
    const asked = images.filter((i) => zoomOf(i) === 1)
    expect(asked).toHaveLength(4)
    expect(root.children).toHaveLength(2)

    // Panned so three of the four leave the view before they arrive.
    layer.lay({ x: 0.5, y: 0.5, w: 0.5, h: 0.5 }, 256, 256)
    const left = asked.filter((i) => !i.parent)
    expect(left).toHaveLength(3)

    // They arrive anyway — a removed image goes on loading. Each one used to
    // count again, and the old level went while the one tile still wanted
    // was on its way, leaving a hole where the basemap was.
    left.forEach((i) => i.fire('load'))
    expect(root.children).toHaveLength(2)

    asked.filter((i) => i.parent).forEach((i) => i.fire('load'))
    expect(root.children).toHaveLength(1)
  })

  test('a tile that fails still lets the old level go', () => {
    const layer = tileLayer(tiles)
    const root = layer.element as unknown as FakeElement
    layer.lay({ x: 0, y: 0, w: 1, h: 1 }, 256, 256)
    images.forEach((i) => i.fire('load'))
    layer.lay({ x: 0, y: 0, w: 0.5, h: 0.5 }, 256, 256)
    images.filter((i) => zoomOf(i) === 1).forEach((i) => i.fire('error'))
    expect(root.children).toHaveLength(1)
  })
})
