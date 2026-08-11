import type { Condition, Field, FilterNode, Group } from './types'

/**
 * Client-side evaluation of the filter tree, so the mock UI behaves like the
 * real thing. In production this compiles to bound SQL predicates on the
 * server — the browser never decides what a person is allowed to see.
 */
export function applyFilter<T extends object>(rows: T[], root: Group, fields: Field[]): T[] {
  if (root.children.length === 0) return rows
  return rows.filter((row) => evaluate(row as Record<string, unknown>, root, fields))
}

function evaluate(row: Record<string, unknown>, node: FilterNode, fields: Field[]): boolean {
  if (node.kind === 'group') {
    if (node.children.length === 0) return true
    return node.join === 'and'
      ? node.children.every((c) => evaluate(row, c, fields))
      : node.children.some((c) => evaluate(row, c, fields))
  }
  return testCondition(row, node, fields)
}

function testCondition(row: Record<string, unknown>, c: Condition, fields: Field[]): boolean {
  const field = fields.find((f) => f.name === c.field)
  if (!field) return true
  const raw = row[c.field]

  switch (c.op) {
    case 'isEmpty': return raw === null || raw === undefined || raw === ''
    case 'isNotEmpty': return !(raw === null || raw === undefined || raw === '')
    case 'anyOf': {
      const vals = asArray(c.value)
      return vals.length === 0 || vals.includes(String(raw))
    }
    case 'noneOf': {
      const vals = asArray(c.value)
      return vals.length === 0 || !vals.includes(String(raw))
    }
    default:
      break
  }

  if (c.value === undefined || c.value === '') return true

  if (field.type === 'date') return testDate(new Date(String(raw)), c)

  if (field.role === 'measure' || field.type === 'number' || field.type === 'decimal') {
    const n = Number(raw)
    switch (c.op) {
      case 'eq': return n === Number(c.value)
      case 'gt': return n > Number(c.value)
      case 'gte': return n >= Number(c.value)
      case 'lt': return n < Number(c.value)
      case 'lte': return n <= Number(c.value)
      case 'between': {
        const [lo, hi] = asArray(c.value).map(Number)
        return (lo === undefined || Number.isNaN(lo) || n >= lo) &&
          (hi === undefined || Number.isNaN(hi) || n <= hi)
      }
      default: return true
    }
  }

  const s = String(raw).toLowerCase()
  const v = String(c.value).toLowerCase()
  switch (c.op) {
    case 'is': return s === v
    case 'isNot': return s !== v
    case 'contains': return s.includes(v)
    case 'startsWith': return s.startsWith(v)
    default: return true
  }
}

/** Relative windows resolve against "now" here; the engine resolves them
 *  against the scheduled run date, so "last month" keeps meaning last month. */
function testDate(d: Date, c: Condition): boolean {
  if (Number.isNaN(d.getTime())) return false
  const now = new Date()

  switch (c.op) {
    case 'inLast': {
      const { n, unit } = (c.value ?? { n: 30, unit: 'day' }) as { n: number; unit: string }
      const days = { day: 1, week: 7, month: 30, quarter: 91 }[unit] ?? 1
      const from = new Date(now)
      from.setDate(from.getDate() - n * days)
      return d >= from && d <= now
    }
    case 'thisMonth':
      return d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth()
    case 'lastMonth': {
      const p = new Date(now.getFullYear(), now.getMonth() - 1, 1)
      return d.getFullYear() === p.getFullYear() && d.getMonth() === p.getMonth()
    }
    case 'thisQuarter':
      return d.getFullYear() === now.getFullYear() &&
        Math.floor(d.getMonth() / 3) === Math.floor(now.getMonth() / 3)
    case 'lastQuarter': {
      const q = Math.floor(now.getMonth() / 3) - 1
      const year = q < 0 ? now.getFullYear() - 1 : now.getFullYear()
      const qq = (q + 4) % 4
      return d.getFullYear() === year && Math.floor(d.getMonth() / 3) === qq
    }
    case 'yearToDate':
      return d.getFullYear() === now.getFullYear() && d <= now
    case 'onOrAfter': return d >= new Date(String(c.value))
    case 'onOrBefore': return d <= new Date(String(c.value))
    case 'between': {
      const [from, to] = asArray(c.value)
      return (!from || d >= new Date(String(from))) && (!to || d <= new Date(String(to)))
    }
    default: return true
  }
}

function asArray(v: unknown): string[] {
  return Array.isArray(v) ? (v as unknown[]).map((x) => (x == null ? '' : String(x))) : []
}
