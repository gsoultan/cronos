import { expect, test } from 'bun:test'
import { document, fromYaml, toYaml, unmodelled } from './yaml'

test('a map of scalars', () => {
  expect(toYaml({ name: 'invoices', fields: 7, hidden: true })).toBe(
    'name: invoices\nfields: 7\nhidden: true')
})

// 20mm is a plain scalar: it contains letters, so nothing reads it as a number.
test('a nested map indents under its key', () => {
  expect(toYaml({ page: { size: 'A4', margins: '20mm' } })).toBe(
    'page:\n  size: A4\n  margins: 20mm')
})

test('a list of maps puts the first key on the dash', () => {
  expect(toYaml({ sources: [{ ref: 'warehouse' }, { ref: 'lake', as: 'events' }] })).toBe(
    'sources:\n  - ref: warehouse\n  - ref: lake\n    as: events')
})

test('a list of scalars', () => {
  expect(toYaml({ columns: ['issued_at', 'status', 'total'] })).toBe(
    'columns:\n  - issued_at\n  - status\n  - total')
})

// A query written as a quoted one-liner is a query nobody can read in the file
// afterwards, and reading the file afterwards is why definitions are files.
test('multi-line text becomes a block scalar', () => {
  expect(toYaml({ query: 'SELECT id\nFROM invoices\nWHERE x = 1\n' })).toBe(
    'query: |\n  SELECT id\n  FROM invoices\n  WHERE x = 1')
})

// Everything here parses successfully as the wrong type if left bare.
test('values that would read back as something else are quoted', () => {
  for (const [given, want] of [
    ['yes', '"yes"'], ['no', '"no"'], ['true', '"true"'], ['null', '"null"'],
    ['2026-07-01', '"2026-07-01"'], ['1.0', '"1.0"'], ['42', '"42"'],
    ['0 6 1 * *', '"0 6 1 * *"'], ['', '""'], [' padded ', '" padded "'],
    ['{{ .row.id }}', '"{{ .row.id }}"'], ['a: b', '"a: b"'], ['#1', '"#1"'],
  ] as const) {
    expect(toYaml({ v: given })).toBe(`v: ${want}`)
  }
})

test('ordinary prose is left alone', () => {
  expect(toYaml({ v: "One statement per customer, mailed on the 1st." }))
    .toBe('v: One statement per customer, mailed on the 1st.')
})

test('quotes and backslashes survive', () => {
  expect(toYaml({ v: 'say "hi" \\ there: now' })).toBe('v: "say \\"hi\\" \\\\ there: now"')
})

// Undefined is how an optional field says it is not set. Emitting `key: null`
// would make the server store an explicit empty where the author wrote nothing.
test('absent values are omitted entirely', () => {
  expect(toYaml({ a: 1, b: undefined, c: null, d: 2 })).toBe('a: 1\nd: 2')
  expect(toYaml({ list: [1, undefined, 2] })).toBe('list:\n  - 1\n  - 2')
})

test('an empty collection is explicit', () => {
  expect(toYaml({ a: [], b: {} })).toBe('a: []\nb: {}')
})

test('the envelope', () => {
  expect(document('Dataset', { name: 'invoices' }, { query: 'SELECT 1' })).toBe(
    'apiVersion: cronos.dev/v1\nkind: Dataset\nmetadata:\n  name: invoices\n' +
    'spec:\n  query: SELECT 1\n')
})

// Reading back. The round-trip is the property that matters: an author's file
// opened in a form and saved unchanged must still be their file.

test('scalars come back typed', () => {
  expect(fromYaml('a: 7\nb: true\nc: hello\nd: "42"\ne: null\nf: 1.5')).toEqual({
    a: 7, b: true, c: 'hello', d: '42', e: null, f: 1.5,
  })
})

test('a DSN is a scalar, not a mapping', () => {
  expect(fromYaml('dsn: postgres://u:p@h:5432/db'))
    .toEqual({ dsn: 'postgres://u:p@h:5432/db' })
})

test('nesting, lists, and maps on a dash', () => {
  expect(fromYaml(
    'spec:\n  sources:\n    - ref: warehouse\n    - ref: lake\n      as: events\n  limits:\n    maxRows: 100\n'))
    .toEqual({ spec: { sources: [{ ref: 'warehouse' }, { ref: 'lake', as: 'events' }], limits: { maxRows: 100 } } })
})

test('a block scalar keeps its lines and its inner indentation', () => {
  expect(fromYaml('query: |\n  SELECT id\n  FROM t\n    WHERE x\nnext: 1'))
    .toEqual({ query: 'SELECT id\nFROM t\n  WHERE x\n', next: 1 })
})

test('comments and document markers are ignored', () => {
  expect(fromYaml('---\n# what this is\nname: x  # trailing\nv: "a # b"\n'))
    .toEqual({ name: 'x', v: 'a # b' })
})

test('flow collections', () => {
  expect(fromYaml('a: []\nb: {}\nc: [x, y]')).toEqual({ a: [], b: {}, c: ['x', 'y'] })
})

// Every value the writer quotes must survive being read.
test('round trip', () => {
  const doc = {
    apiVersion: 'cronos.dev/v1',
    kind: 'Schedule',
    metadata: { name: 'monthly', description: "Rolls up last month's invoices." },
    spec: {
      cron: '0 6 1 * *',
      enabled: true,
      retries: 3,
      query: 'SELECT 1\nFROM t\n',
      columns: ['a', 'b'],
      burst: { bind: { customer_id: '{{ .row.id }}' }, since: '2026-07-01' },
      deliver: [{ via: 'email', to: '{{ .row.email }}' }],
      empty: [],
    },
  }
  expect(fromYaml(toYaml(doc))).toEqual(doc)
})

// A block the rewrite omits entirely is named once, at its root: "this drops
// spec.onFailure" is the sentence somebody can act on, and listing every leaf
// beneath it would bury the other paths in the same list.
test('what a form would drop is named, path by path', () => {
  const stored = fromYaml('spec:\n  a: 1\n  onFailure:\n    retries: 3\n  outs:\n    - x\n    - y\n')
  const rewritten = fromYaml('spec:\n  a: 2\n  outs:\n    - x\n')
  expect(unmodelled(stored, rewritten)).toEqual(['spec.a', 'spec.onFailure', 'spec.outs[1]'])
  expect(unmodelled(stored, stored)).toEqual([])
})

// Every hand-written dataset in the repository declares its fields this way.
test('flow mappings, as real definitions write them', () => {
  expect(fromYaml([
    'fields:',
    '  - {name: id,   type: string, role: dimension, label: Customer}',
    '  - {name: total, type: number, role: measure, aggregate: sum}',
  ].join('\n'))).toEqual({
    fields: [
      { name: 'id', type: 'string', role: 'dimension', label: 'Customer' },
      { name: 'total', type: 'number', role: 'measure', aggregate: 'sum' },
    ],
  })
})

test('a nested flow collection is not split at its own commas', () => {
  expect(fromYaml('a: {b: [1, 2], c: 3}')).toEqual({ a: { b: [1, 2], c: 3 } })
})
