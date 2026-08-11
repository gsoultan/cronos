/**
 * Styles for the shadow root.
 *
 * A string rather than a stylesheet asset, so a host page adds one script tag
 * and is finished. Adopted into the shadow root, which is what makes embedding
 * survivable in both directions: the host's `* { box-sizing: content-box }`
 * cannot reach in, and nothing here leaks out onto their page.
 *
 * Every colour and radius is a custom property with a fallback. Custom
 * properties are the one thing that *does* cross a shadow boundary, so they
 * are the theming API — set them on the element or anywhere above it. There is
 * no `theme` attribute with a list of our opinions.
 */
export const css = `
:host {
  --cr-font: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  --cr-ink: #14140f;
  --cr-ink-secondary: #52514e;
  --cr-ink-muted: #898781;
  --cr-surface: #fff;
  --cr-line: #e4e2dd;
  --cr-accent: #2563eb;
  --cr-good: #15803d;
  --cr-serious: #b91c1c;
  --cr-radius: 8px;
  --cr-gap: 16px;

  display: block;
  font-family: var(--cr-font);
  color: var(--cr-ink);
  font-size: 14px;
  line-height: 1.5;
  container-type: inline-size;
}
:host([hidden]) { display: none }

* { box-sizing: border-box }

.grid {
  display: grid;
  gap: var(--cr-gap);
  grid-template-columns: repeat(auto-fit, minmax(min(220px, 100%), 1fr));
}
.wide { grid-column: 1 / -1 }

.panel {
  background: var(--cr-surface);
  border: 1px solid var(--cr-line);
  border-radius: var(--cr-radius);
  padding: 16px;
  min-width: 0;
}
.panel h3 {
  margin: 0 0 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--cr-ink-secondary);
}

.stat { font-size: 30px; font-weight: 650; letter-spacing: -0.02em; line-height: 1.1 }
.delta { margin-top: 6px; font-size: 13px; color: var(--cr-ink-muted) }
.delta b { font-weight: 600 }
.up { color: var(--cr-good) }
.down { color: var(--cr-serious) }

table { border-collapse: collapse; width: 100%; font-variant-numeric: tabular-nums }
th, td {
  padding: 7px 8px;
  text-align: left;
  border-bottom: 1px solid var(--cr-line);
  white-space: nowrap;
}
th {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--cr-ink-muted);
}
td.r, th.r { text-align: right }
tr:last-child td { border-bottom: 0 }
.scroll { overflow-x: auto; margin: 0 -16px; padding: 0 16px }

/* Not-applicable is a first-class state, not a warning. It reads as
   information about the block, because that is what it is. */
.unaffected {
  margin-top: 10px;
  font-size: 12px;
  color: var(--cr-ink-muted);
}

.msg { padding: 24px; text-align: center; color: var(--cr-ink-muted) }
.msg.err { color: var(--cr-serious) }

.bars { display: grid; gap: 2px }
.bar-row { display: grid; grid-template-columns: minmax(56px, auto) 1fr auto; gap: 10px; align-items: center }
.bar-row span { font-size: 12px; color: var(--cr-ink-secondary) }
.bar-row .v { text-align: right; font-variant-numeric: tabular-nums; color: var(--cr-ink) }
.track { height: 24px; display: flex; align-items: center }
/* 24px cap and a 4px rounded data-end, per the mark spec. A bar that grows
   past 24px stops reading as a bar and starts reading as a block of colour. */
.fill {
  height: 24px;
  background: var(--cr-accent);
  border-radius: 0 4px 4px 0;
  min-width: 2px;
}

@media (prefers-color-scheme: dark) {
  :host {
    --cr-ink: #f5f4f1;
    --cr-ink-secondary: #b9b6ae;
    --cr-ink-muted: #8c8981;
    --cr-surface: #17171a;
    --cr-line: #2c2c31;
    --cr-good: #4ade80;
    --cr-serious: #f87171;
  }
}
`
