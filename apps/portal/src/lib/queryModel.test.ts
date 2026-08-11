import { describe, expect, test } from 'bun:test'
import { emptyQuery, fieldsFromQuery, filterToSql, toSql, type QueryModel } from './queryModel'
import type { Group } from './types'

function base(): QueryModel {
  return {
    ...emptyQuery(),
    from: 'invoices',
    fromAlias: 'i',
    columns: [
      { id: '1', source: 'i.issued_at', alias: 'issued_at' },
      { id: '2', source: 'i.status', alias: 'status' },
    ],
  }
}

describe('toSql', () => {
  test('renders a plain projection without redundant aliases', () => {
    expect(toSql(base())).toBe(
      'SELECT\n  i.issued_at,\n  i.status\nFROM invoices i',
    )
  })

  test('aliases a renamed column', () => {
    const q = base()
    q.columns[0]!.alias = 'when_issued'
    expect(toSql(q)).toContain('i.issued_at AS when_issued')
  })

  test('always aliases an aggregate', () => {
    // Without this the column comes back named `sum` and every field bound to
    // it downstream misses.
    const q = base()
    q.columns.push({ id: '3', source: 'i.total_cents', alias: 'total_cents', agg: 'sum' })
    expect(toSql(q)).toContain('SUM(i.total_cents) AS total_cents')
  })

  test('derives GROUP BY from the non-aggregated columns', () => {
    const q = base()
    q.columns.push({ id: '3', source: 'i.total_cents', alias: 'total', agg: 'sum' })
    expect(toSql(q)).toContain('GROUP BY i.issued_at, i.status')
  })

  test('omits GROUP BY when every column is aggregated', () => {
    const q = base()
    q.columns = [{ id: '1', source: 'i.total_cents', alias: 'total', agg: 'sum' }]
    expect(toSql(q)).not.toContain('GROUP BY')
  })

  test('renders joins with the declared direction', () => {
    const q = base()
    q.joins = [{ id: 'j1', type: 'left', table: 'customers', alias: 'c', leftCol: 'customer_id', rightCol: 'id' }]
    expect(toSql(q)).toContain('LEFT JOIN customers c ON c.id = i.customer_id')
    q.joins[0]!.type = 'inner'
    expect(toSql(q)).toContain('JOIN customers c ON c.id = i.customer_id')
  })

  test('says what to do when nothing is chosen yet', () => {
    expect(toSql(emptyQuery())).toBe('-- Choose a table to start')
  })
})

const group = (join: 'and' | 'or', children: Group['children']): Group =>
  ({ id: 'g', kind: 'group', join, children })

describe('filterToSql', () => {
  test('emits a bound placeholder, never a literal', () => {
    const g = group('and', [
      { id: 'c1', kind: 'condition', field: 'status', op: 'is', value: "'; DROP TABLE invoices; --" },
    ])
    const sql = filterToSql(g)
    expect(sql).toBe('status = {{ .params.status }}')
    expect(sql).not.toContain('DROP')
  })

  test('joins conditions with the group operator and parenthesises', () => {
    const g = group('or', [
      { id: 'c1', kind: 'condition', field: 'status', op: 'is' },
      { id: 'c2', kind: 'condition', field: 'total', op: 'gt' },
    ])
    expect(filterToSql(g)).toBe('(status = {{ .params.status }} OR total > {{ .params.total }})')
  })

  test('nests groups', () => {
    const g = group('and', [
      { id: 'c1', kind: 'condition', field: 'status', op: 'is' },
      group('or', [
        { id: 'c2', kind: 'condition', field: 'a', op: 'is' },
        { id: 'c3', kind: 'condition', field: 'b', op: 'is' },
      ]),
    ])
    expect(filterToSql(g)).toBe(
      '(status = {{ .params.status }} AND (a = {{ .params.a }} OR b = {{ .params.b }}))',
    )
  })

  test('relative dates compile to a run-window range, not to now()', () => {
    const g = group('and', [{ id: 'c1', kind: 'condition', field: 'issued_at', op: 'lastMonth' }])
    const sql = filterToSql(g)
    expect(sql).toBe('issued_at BETWEEN {{ .window.start }} AND {{ .window.end }}')
    expect(sql.toLowerCase()).not.toContain('now(')
  })

  test('an empty group produces no WHERE clause', () => {
    expect(filterToSql(group('and', []))).toBe('')
  })
})

describe('fieldsFromQuery', () => {
  test('classifies numeric columns as measures and the rest as dimensions', () => {
    const q = base()
    q.columns.push({ id: '3', source: 'i.total_cents', alias: 'total_cents' })
    const fields = fieldsFromQuery(q)
    expect(fields.find((f) => f.name === 'total_cents')?.role).toBe('measure')
    expect(fields.find((f) => f.name === 'status')?.role).toBe('dimension')
  })

  test('an aggregate is a measure whatever the underlying column was', () => {
    const q = base()
    q.columns = [{ id: '1', source: 'i.status', alias: 'n', agg: 'count' }]
    const [field] = fieldsFromQuery(q)
    expect(field?.role).toBe('measure')
    expect(field?.type).toBe('number')
  })

  test('resolves a joined table for its column types', () => {
    const q = base()
    q.joins = [{ id: 'j1', type: 'left', table: 'customers', alias: 'c', leftCol: 'customer_id', rightCol: 'id' }]
    q.columns = [{ id: '1', source: 'c.name', alias: 'customer_name' }]
    expect(fieldsFromQuery(q)[0]?.type).toBe('string')
  })
})
