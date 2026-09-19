import { el } from './dom'

/**
 * The hover layer.
 *
 * An HTML/SVG chart is interactive whether or not anyone planned for it, so a
 * mark that says nothing when pointed at reads as broken. One tooltip element
 * per panel, moved and refilled, rather than one per mark: a map with four
 * thousand markers would otherwise build four thousand nodes that are hidden
 * for all but one moment of their lives.
 *
 * Positioned against the panel rather than the page, because the panel is the
 * only box this package controls — a fixed-position tooltip on a host page
 * that has its own transformed ancestor lands somewhere else entirely.
 */
export interface Tips {
  /**
   * Makes target say text when hovered or focused. Call once per mark: it
   * registers listeners, so calling it per pointermove would add a set each
   * time and never remove them.
   */
  bind(target: Element, text: string, sub?: string): void
  /** Shows the tooltip at an event's position. For a mark with no element of
   *  its own — a crosshair reading a column of a line chart. */
  show(e: PointerEvent, text: string, sub?: string): void
  hide(): void
}

export function withTips(panel: HTMLElement): Tips {
  const tip = el('div', { class: 'tip', part: 'tooltip', hidden: '' })
  panel.append(tip)

  const hide = () => tip.setAttribute('hidden', '')
  panel.addEventListener('pointerleave', hide)
  // A scroll under a stationary pointer leaves the tooltip pinned to a mark
  // that has moved out from under it.
  panel.addEventListener('scroll', hide, { passive: true })

  const fill = (text: string, sub?: string) => {
    tip.replaceChildren(el('b', {}, text), ...(sub ? [el('span', {}, sub)] : []))
    tip.removeAttribute('hidden')
  }

  return {
    hide,
    show(e, text, sub) {
      fill(text, sub)
      place(panel, tip, e.clientX, e.clientY)
    },
    bind(target, text, sub) {
      // Focus as well as hover, and a tabindex to make it reachable: a
      // keyboard reader gets the same numbers rather than a chart they can
      // see and not interrogate.
      target.setAttribute('tabindex', '0')
      target.setAttribute('role', 'img')
      target.setAttribute('aria-label', sub ? `${text}, ${sub}` : text)

      const at = (e: Event) => {
        fill(text, sub)
        const p = e as PointerEvent
        // A focus event carries no coordinates. Falling back to the target's
        // own box is what puts the tooltip beside a mark reached by keyboard.
        if (p.clientX) place(panel, tip, p.clientX, p.clientY)
        else {
          const r = target.getBoundingClientRect()
          place(panel, tip, r.left + r.width / 2, r.top)
        }
      }
      target.addEventListener('pointerenter', at)
      target.addEventListener('pointermove', at)
      target.addEventListener('focus', at)
      target.addEventListener('blur', hide)
    },
  }
}

/**
 * Puts the tooltip beside the pointer, and inside the panel.
 *
 * Flipped rather than clamped at the right edge: a tooltip clamped to the edge
 * sits on top of the mark it describes, which hides the thing being asked
 * about at the moment of asking.
 */
function place(panel: HTMLElement, tip: HTMLElement, clientX: number, clientY: number) {
  const box = panel.getBoundingClientRect()
  const x = clientX - box.left
  const y = clientY - box.top
  const flip = x + tip.offsetWidth + 16 > box.width
  tip.style.left = `${flip ? x - tip.offsetWidth - 12 : x + 12}px`
  tip.style.top = `${Math.max(0, y - tip.offsetHeight - 8)}px`
}
