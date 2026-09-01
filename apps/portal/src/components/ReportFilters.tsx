import { useState } from 'react'
import { Button, Select, TextInput } from '@mantine/core'
import type { ReportFilter, RunFilters } from '../lib/api'

/**
 * The filters a report declares, as controls somebody can actually use.
 *
 * The live report had none. A report could declare a Period and a Region, the
 * server would return them in the view, accept values for them on the render
 * call, and compute per-block coverage so the interface could say which numbers
 * a filter did not move — and the connected portal never drew a single control.
 * The panel existed only on the sample path, over a fixture, filtered in a web
 * worker.
 *
 * It went unnoticed for the reason everything else on this page did: until the
 * development script connected the portal to its own API, sample mode was the
 * only view anybody had, and there the panel is present and works.
 *
 * One operator per type, chosen for what a reader means rather than what the
 * engine can express:
 *
 *   - a date is a range, because "the period" is two ends;
 *   - a string is `contains`, because somebody typing "acme" means "find acme",
 *     not "exactly acme in the case I typed";
 *   - an enum is `in`, so the control can grow to multiple selection without
 *     the request shape changing;
 *   - a number and a bool are equality, which is the only thing a single input
 *     can honestly mean.
 *
 * The full operator set — ne, lt, gt, isNull — belongs to the block filter
 * builder, where somebody is authoring rather than reading. Offering eleven
 * operators above a dashboard is a different product.
 *
 * A plain label rather than Field, which marks optional inputs so that the
 * exceptions carry information. Every control on a filter bar is optional, so
 * that rule would put "OPTIONAL" beside every one of them — which is the noise
 * the rule exists to avoid, and the same mistake as a coverage hint under every
 * block of an unfiltered screen.
 */
export function ReportFilters({ filters, value, onApply }: {
  filters: ReportFilter[]
  value: RunFilters
  onApply: (next: RunFilters) => void
}) {
  const [draft, setDraft] = useState<Record<string, string[]>>(() => textOf(value))

  if (filters.length === 0) return null

  const dirty = JSON.stringify(textOf(value)) !== JSON.stringify(draft)
  const anySet = Object.values(draft).some((v) => v.some((s) => s.trim() !== ''))

  function set(name: string, at: number, text: string) {
    setDraft((d) => {
      const pair = [...(d[name] ?? ['', ''])]
      pair[at] = text
      return { ...d, [name]: pair }
    })
  }

  return (
    <section data-testid="report-filters"
      className="mb-6 rounded-lg border border-line bg-surface p-4 shadow-card">
      <div className="flex flex-wrap items-end gap-4">
        {filters.map((f) => (
          <div key={f.name} className="min-w-[180px]">
            <label className="block">
              <span className="mb-1 block text-caption font-medium text-ink-secondary">
                {f.label || f.name}
              </span>
              {f.type === 'date' ? (
                /* Two ends, side by side. A single date control for something
                   called "Period" makes somebody guess whether it means from,
                   to, or on. */
                <div className="flex items-center gap-2">
                  <TextInput type="date" size="xs" aria-label={`${f.label} from`}
                    data-testid={`filter-${f.name}-from`}
                    value={draft[f.name]?.[0] ?? ''}
                    onChange={(e) => set(f.name, 0, e.currentTarget.value)} />
                  <span className="text-caption text-ink-muted">to</span>
                  <TextInput type="date" size="xs" aria-label={`${f.label} to`}
                    data-testid={`filter-${f.name}-to`}
                    value={draft[f.name]?.[1] ?? ''}
                    onChange={(e) => set(f.name, 1, e.currentTarget.value)} />
                </div>
              ) : f.type === 'enum' && f.values?.length ? (
                <Select size="xs" clearable data={f.values} w={180}
                  data-testid={`filter-${f.name}`}
                  placeholder="Any"
                  value={draft[f.name]?.[0] || null}
                  onChange={(v) => set(f.name, 0, v ?? '')} />
              ) : f.type === 'bool' ? (
                <Select size="xs" clearable w={140} data={['true', 'false']}
                  data-testid={`filter-${f.name}`} placeholder="Any"
                  value={draft[f.name]?.[0] || null}
                  onChange={(v) => set(f.name, 0, v ?? '')} />
              ) : (
                <TextInput size="xs" w={180}
                  type={f.type === 'number' ? 'number' : 'text'}
                  data-testid={`filter-${f.name}`}
                  placeholder={f.type === 'number' ? 'Any' : 'Contains…'}
                  value={draft[f.name]?.[0] ?? ''}
                  onChange={(e) => set(f.name, 0, e.currentTarget.value)} />
              )}
            </label>
          </div>
        ))}

        <div className="flex items-center gap-2">
          {/* Applied on a button rather than on every keystroke. A report is a
              query against somebody's warehouse, and a filter bar that re-runs
              as you type is a cost their DBA notices. */}
          <Button size="xs" disabled={!dirty} data-testid="apply-filters"
            onClick={() => onApply(toFilters(filters, draft))}>
            Apply
          </Button>
          {anySet && (
            <Button size="xs" variant="subtle" color="gray" data-testid="clear-filters"
              onClick={() => { setDraft({}); onApply({}) }}>
              Clear
            </Button>
          )}
        </div>
      </div>
    </section>
  )
}

/** The applied filters, back as the text the controls hold. */
function textOf(value: RunFilters): Record<string, string[]> {
  const out: Record<string, string[]> = {}
  for (const [name, v] of Object.entries(value)) {
    out[name] = v.values.map((x) => String(x ?? ''))
  }
  return out
}

/**
 * The controls, as a request.
 *
 * Empty inputs are left out rather than sent as empty strings: `contains ""`
 * matches every row and `between "" ""` is an error, and both would be a filter
 * somebody did not set changing what they see.
 */
export function toFilters(filters: ReportFilter[], draft: Record<string, string[]>): RunFilters {
  const out: RunFilters = {}

  for (const f of filters) {
    const parts = (draft[f.name] ?? []).map((s) => s.trim())

    if (f.type === 'date') {
      const [from, to] = parts
      if (from && to) out[f.name] = { op: 'between', values: [from, to] }
      // One end is still a filter, and the honest operator for it says which
      // end — rather than inventing the other and narrowing more than asked.
      else if (from) out[f.name] = { op: 'gte', values: [from] }
      else if (to) out[f.name] = { op: 'lte', values: [to] }
      continue
    }

    const [first] = parts
    if (!first) continue

    switch (f.type) {
      case 'enum':
        out[f.name] = { op: 'in', values: [first] }
        break
      case 'number':
        out[f.name] = { op: 'eq', values: [Number(first)] }
        break
      case 'bool':
        out[f.name] = { op: 'eq', values: [first === 'true'] }
        break
      default:
        out[f.name] = { op: 'contains', values: [first] }
    }
  }
  return out
}
