import { useState } from 'react'
import { Button, Chip, NumberInput, Radio, Select, TextInput } from '@mantine/core'
import { DatePickerInput } from '@mantine/dates'
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
              <FilterControl f={f} draft={draft} set={set} />
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

/**
 * The control the author asked for.
 *
 * `control` arrives already resolved — the server applies the type's default —
 * so this never guesses. That is the point of it travelling on the wire: a
 * default applied here and again in the embed is two defaults, and the same
 * report would look different in the two places.
 */
function FilterControl({ f, draft, set }: {
  f: ReportFilter
  draft: Record<string, string[]>
  set: (name: string, at: number, v: string) => void
}) {
  const at = (i: number) => draft[f.name]?.[i] ?? ''
  const label = f.label || f.name

  switch (f.control) {
    case 'calendar':
      return (
        <DatePickerInput size="xs" w={180} clearable valueFormat="DD MMM YYYY"
          placeholder="Any date" aria-label={label}
          data-testid={`filter-${f.name}`}
          value={at(0) || null}
          onChange={(v) => set(f.name, 0, v ?? '')} />
      )

    case 'presets':
      /* Relative periods, which is what somebody means by "last 30 days" — a
         calendar makes them work out today's date and count backwards. */
      return (
        <Chip.Group multiple={false} value={at(0)}
          onChange={(v: string | null) => set(f.name, 0, String(v ?? ''))}>
          <div className="flex flex-wrap gap-1.5" data-testid={`filter-${f.name}`}>
            {PRESETS.map((p) => (
              <Chip key={p.value} size="xs" value={p.value}>{p.label}</Chip>
            ))}
          </div>
        </Chip.Group>
      )

    case 'range':
      /* Two ends. Either alone narrows — from with no to is "since", and the
         server compiles it as one. */
      return f.type === 'number' ? (
        <div className="flex items-center gap-2">
          <NumberInput size="xs" w={110} placeholder="From" aria-label={`${label} from`}
            data-testid={`filter-${f.name}-from`}
            value={at(0)} onChange={(v) => set(f.name, 0, String(v ?? ''))} />
          <span className="text-caption text-ink-muted">to</span>
          <NumberInput size="xs" w={110} placeholder="To" aria-label={`${label} to`}
            data-testid={`filter-${f.name}-to`}
            value={at(1)} onChange={(v) => set(f.name, 1, String(v ?? ''))} />
        </div>
      ) : (
        <DatePickerInput type="range" size="xs" w={230} clearable
          valueFormat="DD MMM YYYY" placeholder="Any period" aria-label={label}
          data-testid={`filter-${f.name}`}
          value={[at(0) || null, at(1) || null]}
          onChange={([from, to]) => { set(f.name, 0, from ?? ''); set(f.name, 1, to ?? '') }} />
      )

    case 'radio':
      return (
        <Radio.Group value={at(0)} onChange={(v) => set(f.name, 0, v)}
          aria-label={label}>
          <div className="flex flex-wrap gap-3" data-testid={`filter-${f.name}`}>
            {(f.values ?? ['true', 'false']).map((v) => (
              <Radio key={v} size="xs" value={v} label={v} />
            ))}
          </div>
        </Radio.Group>
      )

    case 'checkboxes':
      /* Several at once, which a dropdown can do and does not show: with four
         statuses the list is shorter than the control that hides it. */
      return (
        <Chip.Group multiple value={draft[f.name] ?? []}
          onChange={(v: string[]) => v.forEach((x, i) => set(f.name, i, x))}>
          <div className="flex flex-wrap gap-1.5" data-testid={`filter-${f.name}`}>
            {(f.values ?? []).map((v) => (
              <Chip key={v} size="xs" value={v}>{v}</Chip>
            ))}
          </div>
        </Chip.Group>
      )

    case 'slider':
      return (
        <NumberInput size="xs" w={180} placeholder="Any" aria-label={label}
          data-testid={`filter-${f.name}`}
          value={at(0)} onChange={(v) => set(f.name, 0, String(v ?? ''))} />
      )

    case 'dropdown':
      return (
        <Select size="xs" clearable w={180} placeholder="Any" aria-label={label}
          data-testid={`filter-${f.name}`}
          data={f.values?.length ? f.values : ['true', 'false']}
          value={at(0) || null}
          onChange={(v) => set(f.name, 0, v ?? '')} />
      )

    default:
      /* search, and anything a newer server sends that this build has not
         learned: a text box narrows by containing, which is the least wrong
         thing to do with a filter whose control is unknown. */
      return (
        <TextInput size="xs" w={180} placeholder="Contains…" aria-label={label}
          data-testid={`filter-${f.name}`}
          value={at(0)} onChange={(e) => set(f.name, 0, e.currentTarget.value)} />
      )
  }
}

/** The relative periods a presets control offers. */
const PRESETS = [
  { value: '7d', label: 'Last 7 days' },
  { value: '30d', label: 'Last 30 days' },
  { value: '90d', label: 'Last 90 days' },
  { value: 'mtd', label: 'Month to date' },
]
