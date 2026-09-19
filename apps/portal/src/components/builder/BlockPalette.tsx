import type { TileKind } from '../../lib/types'

export const PALETTE: { kind: TileKind; label: string; hint: string; icon: string }[] = [
  { kind: 'stat', label: 'Number', hint: 'One headline figure', icon: '#' },
  { kind: 'bar', label: 'Bar chart', hint: 'Compare categories', icon: '▮' },
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
  { kind: 'table', label: 'Table', hint: 'Every row', icon: '▤' },
]

/**
 * A narrow strip rather than a row of cards across the top.
 *
 * The palette is used once per block and then ignored; the canvas is looked at
 * constantly. Giving the palette 72px of width instead of 140px of height is
 * the whole trade, and the canvas gets the difference.
 */
export function BlockPalette({ onAdd }: { onAdd: (kind: TileKind) => void }) {
  return (
    <div data-testid="block-palette"
      className="flex shrink-0 gap-2 max-lg:flex-row max-lg:overflow-x-auto lg:w-[76px] lg:flex-col">
      <p className="hidden text-micro font-semibold tracking-[0.06em] text-ink-muted uppercase lg:block">
        Add
      </p>
      {PALETTE.map((p) => (
        <button key={p.kind} type="button" onClick={() => onAdd(p.kind)}
          title={`${p.label} — ${p.hint}`} aria-label={`Add ${p.label}`}
          className="grid shrink-0 cursor-pointer place-items-center gap-1 rounded-md border
                     border-dashed border-line bg-surface px-2 py-3 text-ink
                     hover:border-solid hover:border-accent hover:bg-accent-wash">
          <span aria-hidden className="text-lead leading-none text-accent">{p.icon}</span>
          <span className="text-center text-micro leading-tight font-medium">{p.label}</span>
        </button>
      ))}
    </div>
  )
}
