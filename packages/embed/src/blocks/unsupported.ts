import { el } from '../dom'

/**
 * A block this build cannot draw.
 *
 * The server and the viewer are deployed separately — a host page pins a
 * version of the bundle and cronos ships a feature months later — so "a block
 * kind I have never heard of" is a normal condition, not a bug. Saying so
 * beats an empty panel, and it very much beats the exception that used to
 * happen when an unknown kind fell through to the table renderer.
 */
export function unsupported(message: string): HTMLElement {
  return el('section', { class: 'panel', part: 'panel' },
    el('p', { class: 'unaffected' }, message))
}
