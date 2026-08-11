import type { Condition, Field, FilterNode, Group } from './types'
import { tableByName } from './schema'

/** What the visual builder edits. Compiles to SQL; never the other way round. */

export type Aggregate = 'sum' | 'avg' | 'count' | 'min' | 'max'

export interface SelectedColumn {
  id: string
  /** Qualified source column, e.g. `i.total_cents`. */
  source: string
  /** Output name. Also the field name every report downstream will bind to. */
  alias: string
  agg?: Aggregate
}

export interface Join {
  id: string
  type: 'inner' | 'left'
  table: string
  alias: string
  /** `on` is expressed as qualified columns on both sides. */
  leftCol: string
  rightCol: string
}

export interface QueryModel {
  from: string
  fromAlias: string
  joins: Join[]
  columns: SelectedColumn[]
  filter: Group
  orderBy: { source: string; dir: 'asc' | 'desc' }[]
  limit?: number
}

export const emptyQuery = (): QueryModel => ({
  from: '',
  fromAlias: '',
  joins: [],
  columns: [],
  filter: { id: 'root', kind: 'group', join: 'and', children: [] },
  orderBy: [],
})

/* -- SQL generation ------------------------------------------------------ */

const AGG_SQL: Record<Aggregate, string> = {
  sum: 'SUM', avg: 'AVG', count: 'COUNT', min: 'MIN', max: 'MAX',
}

/**
 * Renders the model as SQL.
 *
 * Two properties matter more than the formatting. Parameters and row scope come
 * out as `{{ .params.x }}` / `{{ .scope.x }}` placeholders, never as literals —
 * the engine binds those, and a builder that pasted values in would be teaching
 * the wrong thing while quietly building an injection. And grouping is derived,
 * not asked for: if any column is aggregated, every column that is not becomes
 * the GROUP BY, which is the rule people get wrong by hand.
 */
export function toSql(q: QueryModel): string {
  if (!q.from) return '-- Choose a table to start'

  const cols = q.columns.length
    ? q.columns.map((c) => {
        const expr = c.agg ? `${AGG_SQL[c.agg]}(${c.source})` : c.source
        /* An aggregate always gets an alias: `SUM(i.total_cents)` with none
           comes back as a column called `sum`, and every field bound to it
           downstream would miss. A plain column only needs one when renamed. */
        const needsAlias = !!c.agg || (!!c.alias && c.alias !== bare(c.source))
        return `  ${expr}${needsAlias ? ` AS ${c.alias}` : ''}`
      })
    : ['  *']

  const lines = [`SELECT`, cols.join(',\n'), `FROM ${q.from} ${q.fromAlias}`]

  for (const j of q.joins) {
    const kind = j.type === 'left' ? 'LEFT JOIN' : 'JOIN'
    lines.push(`${kind} ${j.table} ${j.alias} ON ${j.alias}.${j.rightCol} = ${q.fromAlias}.${j.leftCol}`)
  }

  const where = filterToSql(q.filter)
  if (where) lines.push(`WHERE ${where}`)

  const grouped = q.columns.filter((c) => c.agg)
  if (grouped.length && grouped.length < q.columns.length) {
    lines.push(`GROUP BY ${q.columns.filter((c) => !c.agg).map((c) => c.source).join(', ')}`)
  }

  if (q.orderBy.length) {
    lines.push(`ORDER BY ${q.orderBy.map((o) => `${o.source} ${o.dir.toUpperCase()}`).join(', ')}`)
  }
  if (q.limit) lines.push(`LIMIT ${q.limit}`)

  return lines.join('\n')
}

const bare = (source: string) => source.split('.').at(-1) ?? source

/** Compiles the filter tree to a WHERE clause of bound placeholders. */
export function filterToSql(node: FilterNode): string {
  if (node.kind === 'group') {
    const parts = node.children.map(filterToSql).filter(Boolean)
    if (!parts.length) return ''
    const joined = parts.join(node.join === 'and' ? ' AND ' : ' OR ')
    return parts.length > 1 ? `(${joined})` : joined
  }
  return conditionToSql(node)
}

function conditionToSql(c: Condition): string {
  const col = c.field
  const p = `{{ .params.${c.field} }}`

  switch (c.op) {
    case 'isEmpty': return `${col} IS NULL`
    case 'isNotEmpty': return `${col} IS NOT NULL`
    case 'is': return `${col} = ${p}`
    case 'isNot': return `${col} <> ${p}`
    case 'contains': return `${col} LIKE ${p}`
    case 'startsWith': return `${col} LIKE ${p}`
    case 'anyOf': return `${col} = ANY(${p})`
    case 'noneOf': return `NOT (${col} = ANY(${p}))`
    case 'eq': return `${col} = ${p}`
    case 'gt': return `${col} > ${p}`
    case 'gte': return `${col} >= ${p}`
    case 'lt': return `${col} < ${p}`
    case 'lte': return `${col} <= ${p}`
    case 'between': return `${col} BETWEEN {{ .params.${c.field}_from }} AND {{ .params.${c.field}_to }}`
    case 'onOrAfter': return `${col} >= ${p}`
    case 'onOrBefore': return `${col} <= ${p}`
    /* Relative windows resolve against the run date, so they compile to a
       bound range rather than to now() — a scheduled "last month" must mean
       the month the run belongs to. */
    case 'inLast':
    case 'thisMonth':
    case 'lastMonth':
    case 'thisQuarter':
    case 'lastQuarter':
    case 'yearToDate':
      return `${col} BETWEEN {{ .window.start }} AND {{ .window.end }}`
    default: return ''
  }
}

/* -- Derived field list -------------------------------------------------- */

/**
 * The typed fields a dataset exposes, seeded from the query. Authors then label
 * and classify them — that is what turns a result set into something a report
 * builder can offer choices from.
 */
export function fieldsFromQuery(q: QueryModel): Field[] {
  return q.columns.map((c) => {
    const [alias, colName] = c.source.split('.')
    const table = q.joins.find((j) => j.alias === alias)?.table ?? q.from
    const type = tableByName(table)?.columns.find((x) => x.name === colName)?.type ?? 'string'
    const measure = !!c.agg || (type === 'number' || type === 'decimal')
    return {
      name: c.alias,
      label: humanize(c.alias),
      type,
      role: measure ? 'measure' : 'dimension',
      /* Counts are whole numbers whatever the underlying column was. */
      ...(c.agg === 'count' ? { type: 'number' as const } : {}),
    }
  })
}

/** `total_cents` → `Total cents`. A starting label, not a final one. */
export function humanize(s: string): string {
  const t = s.replace(/_/g, ' ').trim()
  return t.charAt(0).toUpperCase() + t.slice(1)
}
