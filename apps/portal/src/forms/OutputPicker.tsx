const OUTPUTS = [
  {
    id: 'interactive', label: 'Interactive', icon: '◱',
    hint: 'On screen, filterable. This is what embeds into another application.',
  },
  {
    id: 'pdf', label: 'PDF', icon: '▤',
    hint: 'Print-ready pages with headers, grouping and subtotals. What gets mailed out.',
  },
  {
    id: 'xlsx', label: 'Excel', icon: '▦',
    hint: 'One row per record, so people can carry on working with it.',
  },
]

interface Props {
  value: string[]
  onChange: (next: string[]) => void
  /** Stacked rows instead of a card grid — for a narrow rail. */
  compact?: boolean
}

/**
 * Outputs are checkboxes, not a choice: the product's whole point is that one
 * definition produces all three. Presenting them as alternatives would teach
 * exactly the wrong model.
 */
export function OutputPicker({ value, onChange, compact }: Props) {
  function toggle(id: string) {
    // Interactive is the floor — a report with no output is not a report.
    const next = value.includes(id) ? value.filter((v) => v !== id) : [...value, id]
    onChange(next.length ? next : ['interactive'])
  }

  return (
    <div className={compact
        ? 'grid gap-2'
        : 'grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(210px,1fr))]'}>
      {OUTPUTS.map((o) => {
        const on = value.includes(o.id)
        return (
          <button key={o.id} type="button" role="switch" aria-checked={on}
            onClick={() => toggle(o.id)}
            className={`grid cursor-pointer gap-1 rounded-lg border bg-surface p-4 text-left text-ink transition-colors duration-150 ease-out-quick hover:border-accent ${on ? 'border-accent bg-accent-wash' : 'border-line'}`}>
            <span className="text-title leading-none" aria-hidden>{o.icon}</span>
            <span className="flex justify-between font-semibold">
              {o.label}
              <span className="text-accent" aria-hidden>{on ? '✓' : ''}</span>
            </span>
            <span className="text-small text-ink-secondary">{o.hint}</span>
          </button>
        )
      })}
    </div>
  )
}
