import { el } from '../dom'
import type { Viewport } from './view'

/** How the map asks to be redrawn: `settled` once a gesture has stopped. */
export type Redraw = (settled: boolean) => void

/** The zoom buttons, once they exist, so a redraw can disable the one at a
 *  limit. */
export interface Controls {
  refresh(): void
}

/** How far one click of a button, or one press of a key, zooms. */
const STEP = 2

/**
 * Pan and zoom, on a map embedded in somebody else's page.
 *
 * Cooperative, which is the whole design. A map that takes the scroll wheel
 * traps the reader: they scroll down our customer's page, the cursor passes
 * over a map, and the page stops while the map zooms out to the planet. So a
 * plain wheel scrolls the page and says how to zoom, and zooming wants a held
 * ⌘ or Ctrl — which is also what a trackpad pinch sends, so a pinch just
 * works. On a touch screen one finger scrolls the page and two move the map,
 * for the same reason.
 *
 * Dragging with a mouse moves the map, because nobody drags a page; the
 * buttons and the keyboard do everything the gestures do.
 */
export function controls(stage: HTMLElement, port: Viewport, redraw: Redraw,
  hide: () => void): Controls {

  const hint = el('div', { class: 'geo-hint', role: 'status', 'aria-live': 'polite' })
  let hinted: ReturnType<typeof setTimeout> | undefined
  const say = (text: string) => {
    hint.textContent = text
    hint.classList.add('on')
    clearTimeout(hinted)
    hinted = setTimeout(() => hint.classList.remove('on'), 1600)
  }

  let settle: ReturnType<typeof setTimeout> | undefined
  const moving = () => {
    redraw(false)
    clearTimeout(settle)
    settle = setTimeout(() => redraw(true), 140)
  }

  const zoomIn = button('+', 'Zoom in', () => { port.zoom(STEP); redraw(true) })
  const zoomOut = button('−', 'Zoom out', () => { port.zoom(1 / STEP); redraw(true) })
  const reset = button('⤢', 'Show everything', () => { port.fit(); redraw(true) })
  stage.append(el('div', { class: 'geo-controls', part: 'map-controls' }, zoomIn, zoomOut, reset), hint)

  drag(stage, port, moving, hide)
  wheel(stage, port, moving, say)
  touch(stage, port, moving, say)
  keys(stage, port, redraw)

  return {
    refresh() {
      zoomIn.disabled = !port.can(1)
      zoomOut.disabled = !port.can(-1)
      reset.disabled = !port.moved()
    },
  }
}

function button(text: string, label: string, act: () => void): HTMLButtonElement {
  const b = el('button', { type: 'button', 'aria-label': label, title: label }, text)
  // The press is the button's; the drag that would start on the stage
  // underneath it is not.
  b.addEventListener('pointerdown', (e) => e.stopPropagation())
  b.addEventListener('click', act)
  return b
}

/** A mouse or a pen drags the map; a click that did not move is a click. */
function drag(stage: HTMLElement, port: Viewport, moving: () => void, hide: () => void) {
  let from: { id: number; x: number; y: number } | null = null
  let dragging = false

  stage.addEventListener('pointerdown', (e) => {
    if (e.pointerType === 'touch' || e.button !== 0) return
    from = { id: e.pointerId, x: e.clientX, y: e.clientY }
    dragging = false
  })
  stage.addEventListener('pointermove', (e) => {
    if (!from || e.pointerId !== from.id) return
    if ((e.buttons & 1) === 0) {
      // Released somewhere this element never heard about — outside it,
      // before it had captured the pointer. Without this the next hover
      // across the map dragged it.
      from = null
      dragging = false
      stage.classList.remove('dragging')
      return
    }
    const dx = e.clientX - from.x
    const dy = e.clientY - from.y
    // Three pixels of play, or every click on a dot is a tiny drag.
    if (!dragging && Math.hypot(dx, dy) < 3) return
    if (!dragging) {
      dragging = true
      stage.setPointerCapture(e.pointerId)
      stage.classList.add('dragging')
      hide()
    }
    from = { ...from, x: e.clientX, y: e.clientY }
    port.pan(dx, dy)
    moving()
  })
  const end = (e: PointerEvent) => {
    if (!from || e.pointerId !== from.id) return
    from = null
    stage.classList.remove('dragging')
  }
  stage.addEventListener('pointerup', end)
  stage.addEventListener('pointercancel', end)
  // A drag that ends over a mark would also click it. The click arrives
  // after pointerup, so this is the last place to know it was a drag.
  stage.addEventListener('click', (e) => {
    if (!dragging) return
    dragging = false
    // Immediate: a painted place is picked by a click listener on the stage
    // itself, which stopPropagation leaves running.
    e.stopImmediatePropagation()
  }, true)
}

/** ⌘ or Ctrl and the wheel zooms about the cursor. A trackpad pinch sends
 *  exactly that, so it zooms too. */
function wheel(stage: HTMLElement, port: Viewport, moving: () => void, say: (t: string) => void) {
  const mac = /Mac|iPhone|iPad/.test(globalThis.navigator?.userAgent ?? '')
  stage.addEventListener('wheel', (e) => {
    if (!e.ctrlKey && !e.metaKey) {
      say(mac ? 'Hold ⌘ and scroll to zoom the map' : 'Hold Ctrl and scroll to zoom the map')
      return
    }
    e.preventDefault()
    // Lines are about sixteen pixels; a notch of a mouse wheel is a hundred,
    // and a pinch a few. The exponent makes both feel like one control.
    const delta = e.deltaY * (e.deltaMode === 1 ? 16 : 1)
    const box = stage.getBoundingClientRect()
    port.zoom(Math.exp(-delta * 0.0025), e.clientX - box.left, e.clientY - box.top)
    moving()
  }, { passive: false })
}

/** Two fingers move and pinch the map; one scrolls the page. */
function touch(stage: HTMLElement, port: Viewport, moving: () => void, say: (t: string) => void) {
  let last: { x: number; y: number; d: number } | null = null
  const read = (t: TouchList) => {
    const a = t[0]
    const b = t[1]
    if (!a || !b) return null
    return {
      x: (a.clientX + b.clientX) / 2, y: (a.clientY + b.clientY) / 2,
      d: Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY),
    }
  }
  stage.addEventListener('touchstart', (e) => {
    last = e.touches.length >= 2 ? read(e.touches) : null
  }, { passive: true })
  stage.addEventListener('touchmove', (e) => {
    if (e.touches.length < 2) {
      say('Use two fingers to move the map')
      return
    }
    e.preventDefault()
    const now = read(e.touches)
    if (!now) return
    if (last) {
      const box = stage.getBoundingClientRect()
      port.pan(now.x - last.x, now.y - last.y)
      if (last.d > 0) port.zoom(now.d / last.d, now.x - box.left, now.y - box.top)
      moving()
    }
    last = now
  }, { passive: false })
  stage.addEventListener('touchend', (e) => {
    last = e.touches.length >= 2 ? read(e.touches) : null
  })
}

/** The keyboard does what the pointer does, once the map has focus. */
function keys(stage: HTMLElement, port: Viewport, redraw: Redraw) {
  stage.addEventListener('keydown', (e) => {
    if (e.target !== stage) return // a focused mark or button keeps its own keys
    const box = stage.getBoundingClientRect()
    const nudge = Math.min(box.width, box.height) / 5
    switch (e.key) {
      case '+': case '=': port.zoom(STEP); break
      case '-': case '_': port.zoom(1 / STEP); break
      case '0': port.fit(); break
      case 'ArrowLeft': port.pan(nudge, 0); break
      case 'ArrowRight': port.pan(-nudge, 0); break
      case 'ArrowUp': port.pan(0, nudge); break
      case 'ArrowDown': port.pan(0, -nudge); break
      default: return
    }
    e.preventDefault()
    redraw(true)
  })
}
