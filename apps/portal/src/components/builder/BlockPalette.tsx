import type { TileKind } from '../../lib/types'

export const PALETTE: { kind: TileKind; label: string; hint: string; icon: string }[] = [
  { kind: 'stat', label: 'Number', hint: 'One headline figure', icon: '#' },
  { kind: 'bar', label: 'Bar chart', hint: 'Compare categories', icon: '☰' },
  { kind: 'column', label: 'Column chart', hint: 'Categories left to right', icon: '▮' },
  { kind: 'line', label: 'Line chart', hint: 'Change over time', icon: '⟋' },
  { kind: 'area', label: 'Area chart', hint: 'A total, over time', icon: '◺' },
  { kind: 'pie', label: 'Pie chart', hint: 'Parts of a whole', icon: '◕' },
  { kind: 'donut', label: 'Donut chart', hint: 'Parts of a whole', icon: '◎' },
  { kind: 'scatter', label: 'Scatter', hint: 'One measure against another', icon: '∷' },
  { kind: 'bubble', label: 'Bubble', hint: 'Three measures at once', icon: '◍' },
  { kind: 'map', label: 'Map', hint: 'Regions, points and flows', icon: '◈' },
  { kind: 'combo', label: 'Bars + line', hint: 'Two measures together', icon: '⊞' },
  { kind: 'funnel', label: 'Funnel', hint: 'Where people drop out', icon: '⧩' },
  { kind: 'waterfall', label: 'Waterfall', hint: 'What moved a total', icon: '⊪' },
  { kind: 'heatmap', label: 'Heatmap', hint: 'Two dimensions at once', icon: '▦' },
  { kind: 'gauge', label: 'Gauge', hint: 'Against a target', icon: '◔' },
  { kind: 'treemap', label: 'Treemap', hint: 'Parts within parts', icon: '▧' },
  { kind: 'radar', label: 'Radar', hint: 'A profile across measures', icon: '✳' },
  { kind: 'bullet', label: 'Bullet', hint: 'Each against a target', icon: '⊢' },
  { kind: 'histogram', label: 'Histogram', hint: 'How a number spreads', icon: '▙' },
  { kind: 'boxplot', label: 'Box plot', hint: 'Its spread, by category', icon: '⧈' },
  { kind: 'sankey', label: 'Sankey', hint: 'Where it flows', icon: '⫘' },
  { kind: 'sunburst', label: 'Sunburst', hint: 'Parts within parts, as rings', icon: '◉' },
  { kind: 'calendar', label: 'Calendar', hint: 'Day by day, a year at a time', icon: '▣' },
  { kind: 'pivot', label: 'Pivot', hint: 'One field against another, with totals', icon: '⊟' },
  { kind: 'table', label: 'Table', hint: 'Every row', icon: '▤' },
]

/*
 * The same kinds, grouped by what somebody wants the block to show. Twenty-five
 * entries in one column ran past the bottom of the editor and had to be
 * scrolled to find a table; in six groups of two-across they fit, and the
 * group name answers the question the author arrived with.
 */
export const GROUPS: { name: string; kinds: TileKind[] }[] = [
  { name: 'Numbers and tables', kinds: ['stat', 'table', 'pivot', 'gauge'] },
  { name: 'Compare', kinds: ['bar', 'column', 'combo', 'bullet', 'radar', 'heatmap'] },
  { name: 'Over time', kinds: ['line', 'area', 'waterfall', 'calendar'] },
  { name: 'Parts of a whole', kinds: ['pie', 'donut', 'treemap', 'sunburst', 'funnel'] },
  { name: 'Spread', kinds: ['scatter', 'bubble', 'histogram', 'boxplot'] },
  { name: 'Maps and flows', kinds: ['map', 'sankey'] },
]

/**
 * A column beside the canvas rather than a row of cards across the top.
 *
 * The palette is used once per block and then ignored; the canvas is looked at
 * constantly, so the palette takes width and never height. Each button names
 * its kind without "chart" — the group heading already says it is one — and
 * keeps the full name for its accessible label and tooltip.
 */
export function BlockPalette({ onAdd }: { onAdd: (kind: TileKind) => void }) {
  return (
    <div data-testid="block-palette"
      className="flex shrink-0 gap-3 [scrollbar-width:thin] max-lg:overflow-x-auto
                 lg:min-h-0 lg:w-[168px] lg:flex-col lg:overflow-y-auto">
      {GROUPS.map((g) => (
        <section key={g.name} className="shrink-0">
          <h3 className="mb-1 text-micro font-semibold tracking-[0.06em] whitespace-nowrap
                         text-ink-muted uppercase">
            {g.name}
          </h3>
          <div className="grid grid-cols-2 gap-1 max-lg:flex">
            {g.kinds.map((kind) => {
              const p = PALETTE.find((x) => x.kind === kind)!
              return (
                <button key={kind} type="button" onClick={() => onAdd(kind)}
                  title={`${p.label} — ${p.hint}`} aria-label={`Add ${p.label}`}
                  className="flex min-w-0 cursor-pointer items-center gap-1 rounded-md border
                             border-line bg-surface px-1 py-1.5 text-left text-ink
                             hover:border-accent hover:bg-accent-wash max-lg:shrink-0">
                  <span aria-hidden className="w-3 shrink-0 text-center leading-none text-accent">
                    {p.icon}
                  </span>
                  <span className="truncate text-micro font-medium">
                    {p.label.replace(/ chart$/, '').replace('Bars + line', 'Bar + line')}
                  </span>
                </button>
              )
            })}
          </div>
        </section>
      ))}
    </div>
  )
}
