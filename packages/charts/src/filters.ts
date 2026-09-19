import { el } from './dom'
import type { FilterDef, FilterValues } from './types'

/**
 * The filter bar.
 *
 * The report format has always called these "one control on a report's filter
 * bar" — the definition, the coverage notes a block carries, and the `filters`
 * property all assumed one existed. Nothing drew it, so every host page built
 * its own and reimplemented which operator a date range sends.
 *
 * Controls are rebuilt from the values on every render rather than holding
 * their own state, so a host that sets `.filters` programmatically and a person
 * using the bar cannot disagree about what is applied.
 */
export function filterBar(
  defs: FilterDef[],
  values: FilterValues,
  onChange: (next: FilterValues) => void,
): HTMLElement | null {
  if (defs.length === 0) return null

  const set = (name: string, v: FilterValues[string] | undefined) => {
    const next = { ...values }
    // Removed, not set to an empty list: "no value" and "match nothing" are
    // different requests, and the server reads an absent filter as the first.
    if (v === undefined) delete next[name]
    else next[name] = v
    onChange(next)
  }

  const bar = el('div', { class: 'filters', part: 'filters', role: 'group' })
  for (const def of defs) {
    bar.append(el('label', { class: 'filter' },
      el('span', {}, def.label || def.name),
      control(def, values[def.name], (v) => set(def.name, v))))
  }
  return bar
}

type Value = FilterValues[string] | undefined
type Emit = (v: Value) => void

/**
 * The control the author asked for.
 *
 * Switched on `control`, not on `type`. The server resolves the default, so an
 * author who named none still arrives here with one — and the portal, which
 * reads the same field, draws the same filter. Guessing from the type in two
 * places is two guesses.
 */
function control(def: FilterDef, value: Value, emit: Emit): HTMLElement {
  switch (def.control ?? fallback(def.type)) {
    case 'dropdown':
      return def.values?.length ? choice(def, value, emit) : truth(value, emit)
    case 'radio':
      return chips(def.values ?? ['true', 'false'], value, emit, false)
    case 'checkboxes':
      return chips(def.values ?? [], value, emit, true)
    case 'presets':
      return chips(PRESETS, value, emit, false)
    case 'calendar':
      return one(def, value, emit)
    case 'range':
      return range(def, value, emit, def.type === 'number' ? 'number' : 'date')
    case 'slider':
      return one(def, value, emit, 'number')
    default:
      return text(def, value, emit)
  }
}

/** What a payload from a server too old to send a control gets. */
function fallback(type: string): string {
  if (type === 'enum' || type === 'bool') return 'dropdown'
  if (type === 'date' || type === 'number') return 'range'
  return 'search'
}

/** The relative periods a presets control offers. */
const PRESETS = ['7d', '30d', '90d', 'mtd']
const PRESET_LABELS: Record<string, string> = {
  '7d': 'Last 7 days', '30d': 'Last 30 days', '90d': 'Last 90 days', mtd: 'Month to date',
}

/**
 * A row of toggles, one or several choosable.
 *
 * Buttons rather than checkboxes and radios: with four values the list is
 * shorter than the control that would hide it, and a row of toggles is one
 * tab stop per option either way.
 */
function chips(values: string[], value: Value, emit: Emit, many: boolean): HTMLElement {
  const on = (value?.values ?? []).map(String)
  const row = el('span', { class: 'chips', role: many ? 'group' : 'radiogroup' })

  for (const v of values) {
    const picked = on.includes(v)
    const chip = el('button', {
      type: 'button',
      class: picked ? 'chip on' : 'chip',
      'aria-pressed': String(picked),
    }, PRESET_LABELS[v] ?? v)

    chip.addEventListener('click', () => {
      const next = many
        ? (picked ? on.filter((x) => x !== v) : [...on, v])
        : (picked ? [] : [v])
      emit(next.length ? { op: 'in', values: next } : undefined)
    })
    row.append(chip)
  }
  return row
}

/** One value, for the controls that take exactly one. */
function one(def: FilterDef, value: Value, emit: Emit, kind = 'date'): HTMLElement {
  const node = el('input', {
    type: kind,
    value: String(value?.values?.[0] ?? ''),
    'aria-label': def.label || def.name,
  })
  node.addEventListener('change', () => {
    const v = kind === 'number' ? Number(node.value) : node.value
    emit(node.value ? { op: 'eq', values: [v] } : undefined)
  })
  return node
}

/** An enum sends `in`, so widening it to a multi-select later is not a wire
 *  change — the server already accepts a list. */
function choice(def: FilterDef, value: Value, emit: Emit): HTMLElement {
  const node = el('select', { 'aria-label': def.label || def.name })
  node.append(el('option', { value: '' }, 'Any'))
  for (const v of def.values ?? []) {
    const option = el('option', { value: v }, v)
    if (String(value?.values?.[0] ?? '') === v) option.setAttribute('selected', '')
    node.append(option)
  }
  node.addEventListener('change', () =>
    emit(node.value ? { op: 'in', values: [node.value] } : undefined))
  return node
}

function truth(value: Value, emit: Emit): HTMLElement {
  const node = el('select', {})
  for (const [v, label] of [['', 'Any'], ['true', 'Yes'], ['false', 'No']]) {
    const option = el('option', { value: v! }, label!)
    if (String(value?.values?.[0] ?? '') === v) option.setAttribute('selected', '')
    node.append(option)
  }
  node.addEventListener('change', () =>
    emit(node.value ? { op: 'eq', values: [node.value === 'true'] } : undefined))
  return node
}

/**
 * A from/to pair, for the types that have an order.
 *
 * Either end alone is a valid narrowing, so the operator follows what was
 * filled in: both is `between`, a start alone is `gte`, an end alone is `lte`.
 * Picking `between` regardless and defaulting the empty end is how a report
 * silently excludes everything before 1970.
 */
function range(def: FilterDef, value: Value, emit: Emit, kind: 'date' | 'number'): HTMLElement {
  const [lo, hi] = ends(value)
  const from = el('input', { type: kind, value: lo, 'aria-label': `${def.label || def.name} from` })
  const to = el('input', { type: kind, value: hi, 'aria-label': `${def.label || def.name} to` })

  const send = () => {
    const a = from.value
    const b = to.value
    const cast = (v: string) => (kind === 'number' ? Number(v) : v)
    if (a && b) return emit({ op: 'between', values: [cast(a), cast(b)] })
    if (a) return emit({ op: 'gte', values: [cast(a)] })
    if (b) return emit({ op: 'lte', values: [cast(b)] })
    emit(undefined)
  }
  from.addEventListener('change', send)
  to.addEventListener('change', send)
  return el('span', { class: 'pair' }, from, el('i', {}, '–'), to)
}

/** The two ends a value already carries, whichever operator it used. */
function ends(value: Value): [string, string] {
  const vs = (value?.values ?? []).map(String)
  switch (value?.op) {
    case 'between':
      return [vs[0] ?? '', vs[1] ?? '']
    case 'gte':
      return [vs[0] ?? '', '']
    case 'lte':
      return ['', vs[0] ?? '']
  }
  return ['', '']
}

/** Free text matches on `contains`, which is what a search box means. */
function text(def: FilterDef, value: Value, emit: Emit): HTMLElement {
  const node = el('input', {
    type: 'search',
    value: String(value?.values?.[0] ?? ''),
    placeholder: 'Any',
    'aria-label': def.label || def.name,
  })
  // On change, not on input. Every keystroke is a request otherwise, and the
  // report behind it is a query against somebody's warehouse.
  node.addEventListener('change', () =>
    emit(node.value ? { op: 'contains', values: [node.value] } : undefined))
  return node
}
