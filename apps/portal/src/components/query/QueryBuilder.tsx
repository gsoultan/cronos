import { ActionIcon, Button, NumberInput, Select, TextInput } from '@mantine/core'
import { FilterGroup } from '../FilterGroup'
import { aliasFor, suggestedJoins, TABLES, tableByName } from '../../lib/schema'
import { humanize, type Aggregate, type QueryModel, type SelectedColumn } from '../../lib/queryModel'
import type { Field, Group } from '../../lib/types'

interface Props {
  query: QueryModel
  onChange: (q: QueryModel) => void
}

const AGGREGATES = [
  { value: '', label: 'Each row' },
  { value: 'sum', label: 'Total' },
  { value: 'avg', label: 'Average' },
  { value: 'count', label: 'Count' },
  { value: 'min', label: 'Lowest' },
  { value: 'max', label: 'Highest' },
]

let seq = 0
const nextId = () => `q${++seq}`

/**
 * Building the query without writing SQL.
 *
 * The order is the order a person thinks in — what am I looking at, what else
 * should come with it, which columns, which rows — which is deliberately not
 * the order SQL is written in. Grouping is never asked about: it is derived
 * from whether any column is aggregated, because "you must list every
 * non-aggregated column in GROUP BY" is a rule about SQL, not about the
 * question being asked.
 */
export function QueryBuilder({ query, onChange }: Props) {
  const patch = (p: Partial<QueryModel>) => onChange({ ...query, ...p })

  /* Every column reachable from the query, as `alias.column`. */
  const available: { value: string; label: string; group: string }[] = []
  if (query.from) {
    for (const c of tableByName(query.from)?.columns ?? []) {
      available.push({
        value: `${query.fromAlias}.${c.name}`,
        label: humanize(c.name),
        group: tableByName(query.from)!.label,
      })
    }
  }
  for (const j of query.joins) {
    for (const c of tableByName(j.table)?.columns ?? []) {
      available.push({
        value: `${j.alias}.${c.name}`,
        label: humanize(c.name),
        group: tableByName(j.table)?.label ?? j.table,
      })
    }
  }

  const grouped = [...new Set(available.map((a) => a.group))].map((g) => ({
    group: g,
    items: available.filter((a) => a.group === g).map(({ value, label }) => ({ value, label })),
  }))

  function setFrom(table: string | null) {
    if (!table) return
    const alias = aliasFor(table, [])
    const cols = (tableByName(table)?.columns ?? []).slice(0, 6).map<SelectedColumn>((c) => ({
      id: nextId(), source: `${alias}.${c.name}`, alias: c.name,
    }))
    /* Changing the table invalidates every join, column and filter that
       referenced the old one, so they go rather than dangle. */
    onChange({ ...query, from: table, fromAlias: alias, joins: [], columns: cols,
      filter: { id: 'root', kind: 'group', join: 'and', children: [] }, orderBy: [] })
  }

  function addJoin(s: { table: string; left: string; right: string }) {
    const alias = aliasFor(s.table, [query.fromAlias, ...query.joins.map((j) => j.alias)])
    patch({
      joins: [...query.joins, {
        id: nextId(), type: 'left', table: s.table, alias,
        leftCol: s.left, rightCol: s.right,
      }],
    })
  }

  const suggestions = query.from
    ? suggestedJoins(query.from).filter((s) => !query.joins.some((j) => j.table === s.table))
    : []

  /* Filters run against the query's own output columns, so the filter builder
     is handed the aliases the query produces — not the raw table columns. */
  const filterFields: Field[] = query.columns.map((c) => ({
    name: c.alias, label: humanize(c.alias), type: 'string', role: 'dimension',
  }))

  return (
    <div className="grid gap-4">
      <Step n={1} title="What are you looking at?"
        hint="One table to start from. You can bring in related tables next.">
        <Select data={TABLES.map((t) => ({ value: t.name, label: t.label }))}
          value={query.from || null} onChange={setFrom} allowDeselect={false}
          placeholder="Choose a table" w={280} aria-label="Table" />
        {query.from && (
          <p className="mt-2 text-small text-ink-muted">
            {tableByName(query.from)!.rows.toLocaleString('en')} rows
          </p>
        )}
      </Step>

      {query.from && (
        <Step n={2} title="Bring in related data"
          hint="cronos already knows how these tables connect, so this is a choice, not a formula.">
          {query.joins.length > 0 && (
            <ul className="mb-3 grid gap-2">
              {query.joins.map((j) => (
                <li key={j.id}
                  className="flex flex-wrap items-center gap-2 rounded-md border border-line
                             bg-sunken px-3 py-2 text-small">
                  <Select size="xs" w={140} allowDeselect={false} value={j.type}
                    data={[{ value: 'left', label: 'Include all' }, { value: 'inner', label: 'Only matching' }]}
                    onChange={(t) => patch({
                      joins: query.joins.map((x) =>
                        x.id === j.id ? { ...x, type: (t ?? 'left') as Join['type'] } : x),
                    })} />
                  <span className="font-medium">{tableByName(j.table)?.label ?? j.table}</span>
                  <span className="font-mono text-caption text-ink-muted">
                    {j.alias}.{j.rightCol} = {query.fromAlias}.{j.leftCol}
                  </span>
                  <ActionIcon variant="subtle" color="gray" size="sm" className="ml-auto"
                    aria-label={`Remove ${j.table}`}
                    onClick={() => patch({ joins: query.joins.filter((x) => x.id !== j.id) })}>
                    ✕
                  </ActionIcon>
                </li>
              ))}
            </ul>
          )}
          {suggestions.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {suggestions.map((s) => (
                <button key={s.table} type="button" onClick={() => addJoin(s)}
                  className="cursor-pointer rounded-full border border-line bg-surface px-3 py-1.5
                             text-small text-ink hover:border-accent">
                  + {tableByName(s.table)?.label ?? s.table}
                </button>
              ))}
            </div>
          ) : (
            <p className="text-small text-ink-muted">
              Nothing else connects to this table.
            </p>
          )}
        </Step>
      )}

      {query.from && (
        <Step n={3} title="Which columns?"
          hint="Rename them here — these names are what report authors will see.">
          <ul className="grid gap-2">
            {query.columns.map((c) => (
              <li key={c.id} className="flex flex-wrap items-center gap-2">
                <Select size="xs" w={220} allowDeselect={false} searchable data={grouped}
                  value={c.source} aria-label="Column"
                  onChange={(v) => v && patch({
                    columns: query.columns.map((x) =>
                      x.id === c.id ? { ...x, source: v, alias: v.split('.').at(-1)! } : x),
                  })} />
                <Select size="xs" w={140} allowDeselect={false} data={AGGREGATES}
                  value={c.agg ?? ''} aria-label="Summarise"
                  onChange={(v) => patch({
                    columns: query.columns.map((x) =>
                      x.id === c.id ? { ...x, agg: (v || undefined) as Aggregate | undefined } : x),
                  })} />
                <span className="text-small text-ink-muted">as</span>
                <TextInput size="xs" w={180} value={c.alias} aria-label="Output name"
                  onChange={(e) => patch({
                    columns: query.columns.map((x) =>
                      x.id === c.id ? { ...x, alias: e.currentTarget.value } : x),
                  })} />
                <ActionIcon variant="subtle" color="gray" size="sm" className="ml-auto"
                  aria-label="Remove column"
                  onClick={() => patch({ columns: query.columns.filter((x) => x.id !== c.id) })}>
                  ✕
                </ActionIcon>
              </li>
            ))}
          </ul>
          <Button variant="subtle" size="xs" mt={8}
            onClick={() => {
              const first = available.find((a) => !query.columns.some((c) => c.source === a.value))
              if (first) {
                patch({ columns: [...query.columns, {
                  id: nextId(), source: first.value, alias: first.value.split('.').at(-1)!,
                }] })
              }
            }}>
            + Add column
          </Button>
          {query.columns.some((c) => c.agg) && (
            <p className="mt-3 rounded-r-md border-l-2 border-accent bg-sunken px-3 py-2
                          text-small text-ink-secondary">
              Because something is summarised, the columns that are not become the
              grouping — one row per unique combination of them.
            </p>
          )}
        </Step>
      )}

      {query.from && query.columns.length > 0 && (
        <Step n={4} title="Which rows?"
          hint="Optional. Leave it empty to include everything, and filter in the report instead.">
          <FilterGroup group={query.filter} fields={filterFields}
            onChange={(g: Group) => patch({ filter: g })} />
        </Step>
      )}

      {query.from && query.columns.length > 0 && (
        <Step n={5} title="Order and limit" hint="Optional.">
          <div className="flex flex-wrap items-end gap-3">
            <Select size="sm" w={220} clearable searchable label="Sort by" data={grouped}
              value={query.orderBy[0]?.source ?? null}
              onChange={(v) => patch({ orderBy: v ? [{ source: v, dir: query.orderBy[0]?.dir ?? 'desc' }] : [] })} />
            {query.orderBy[0] && (
              <Select size="sm" w={150} allowDeselect={false} label="Direction"
                data={[{ value: 'desc', label: 'Highest first' }, { value: 'asc', label: 'Lowest first' }]}
                value={query.orderBy[0].dir}
                onChange={(d) => patch({ orderBy: [{ ...query.orderBy[0]!, dir: (d ?? 'desc') as 'asc' | 'desc' }] })} />
            )}
            <NumberInput size="sm" w={160} label="Row limit" min={1} placeholder="No limit"
              value={query.limit ?? ''}
              onChange={(v) => patch({ limit: v === '' ? undefined : Number(v) })} />
          </div>
        </Step>
      )}
    </div>
  )
}

type Join = QueryModel['joins'][number]

function Step({
  n, title, hint, children,
}: { n: number; title: string; hint: string; children: React.ReactNode }) {
  return (
    <section className="rounded-lg border border-line bg-surface p-4">
      <div className="mb-3 flex items-start gap-3">
        <span aria-hidden className="grid size-6 shrink-0 place-items-center rounded-full
                                     bg-sunken text-caption font-semibold text-ink-secondary">
          {n}
        </span>
        <div>
          <h3 className="text-body font-semibold text-ink">{title}</h3>
          <p className="mt-0.5 text-small text-ink-secondary">{hint}</p>
        </div>
      </div>
      <div className="pl-9">{children}</div>
    </section>
  )
}
