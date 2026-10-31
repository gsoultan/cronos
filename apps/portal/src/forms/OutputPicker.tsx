const OUTPUTS = [
  {
    id: 'interactive', label: 'Interactive', icon: '◱',
    hint: 'On screen and filterable; this is what embeds.',
  },
  {
    id: 'pdf', label: 'PDF', icon: '▤',
    hint: 'Printed pages with grouping and subtotals.',
  },
  {
    id: 'xlsx', label: 'Excel', icon: '▦',
    hint: 'A workbook, one row per record.',
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

  /* In the editor's rail, a checklist: three cards there took the whole
     height of the inspector before anybody reached the filters below them. */
  if (compact) {
    return (
      <div className="grid gap-1">
        {OUTPUTS.map((o) => {
          const on = value.includes(o.id)
          return (
            <button key={o.id} type="button" role="switch" aria-checked={on}
              onClick={() => toggle(o.id)}
              className="flex cursor-pointer items-start gap-2.5 rounded-md px-1 py-1.5 text-left
                         text-ink hover:bg-hover">
              <span aria-hidden className={`mt-0.5 grid size-4 shrink-0 place-items-center
                rounded-sm border text-micro leading-none ${on
                  ? 'border-accent bg-accent text-white' : 'border-line bg-surface'}`}>
                {on ? '✓' : ''}
              </span>
              <span className="grid">
                <span className="font-medium">{o.label}</span>
                <span className="text-caption text-ink-secondary">{o.hint}</span>
              </span>
            </button>
          )
        })}
      </div>
    )
  }

  return (
    <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(210px,1fr))]">
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
