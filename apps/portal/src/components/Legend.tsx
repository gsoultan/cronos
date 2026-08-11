export interface LegendItem {
  label: string
  color: string
}

/**
 * Identity is never colour alone: the swatch sits beside a text label, and the
 * label wears a text token rather than the series colour.
 */
export function Legend({ items }: { items: LegendItem[] }) {
  return (
    <ul className="mt-3 flex flex-wrap gap-4">
      {items.map((it) => (
        <li key={it.label} className="flex items-center gap-2 text-small text-ink-secondary">
          <span className="size-2.5 shrink-0 rounded-[3px]"
            style={{ background: it.color }} aria-hidden />
          {it.label}
        </li>
      ))}
    </ul>
  )
}
