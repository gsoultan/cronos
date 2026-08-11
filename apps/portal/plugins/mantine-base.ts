import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import type { Plugin } from 'vite'

const VIRTUAL = 'virtual:mantine-base.css'
const RESOLVED = '\0' + VIRTUAL

/**
 * Serves Mantine's base layer — the reset, `--mantine-scale`, and the
 * colour-scheme variables — without the ~180 KB of component rules bundled
 * alongside them in `@mantine/core/styles.css`.
 *
 * Those variables are not optional: every component rule is built on
 * `calc(… * var(--mantine-scale))`, and an undefined custom property makes the
 * whole declaration invalid, so importing component CSS without the base
 * silently renders unstyled controls.
 *
 * Reads the layered stylesheet, so the base lands in `@layer mantine` and sits
 * below Tailwind's utilities.
 *
 * The split point is the first `.m_<hash>` rule — component styles all carry
 * that prefix — so this tracks upstream instead of pinning a line number.
 */
export function mantineBase(): Plugin {
  return {
    name: 'cronos:mantine-base',
    resolveId: (id) => (id === VIRTUAL ? RESOLVED : null),
    load(id) {
      if (id !== RESOLVED) return null
      const require = createRequire(import.meta.url)
      const css = readFileSync(require.resolve('@mantine/core/styles.layer.css'), 'utf8')
      const cut = css.search(/^\.m_[0-9a-f]+\s*[,{]/m)
      if (cut === -1) {
        this.error('mantine-base: no component rule found — has the CSS layout changed?')
      }
      // The layered file opens with `@layer mantine {` and closes at EOF, so
      // slicing mid-file leaves the block open. Close it.
      return `${css.slice(0, cut)}\n}\n`
    },
  }
}
