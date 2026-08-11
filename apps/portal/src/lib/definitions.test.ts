import { expect, test } from 'bun:test'
import {
  dataset, dataSource, readDataset, readDataSource, readReport, readSchedule, report, schedule,
  withCarry,
} from './definitions'
import type { Field } from './types'

const fields: Field[] = [
  { name: 'id', type: 'string', role: 'dimension', hidden: true },
  { name: 'customer_id', type: 'string', role: 'dimension', hidden: true },
  { name: 'issued_at', type: 'date', role: 'dimension', label: 'Issued' },
  { name: 'total', type: 'decimal', role: 'measure', label: 'Amount' },
] as Field[]

test('a dataset carries its query as a block scalar', () => {
  const yaml = dataset({
    name: 'Invoices', slug: 'invoices', description: 'Issued invoices.',
    source: 'warehouse', fields,
    query: 'SELECT id, customer_id, issued_at, total\nFROM invoices',
  })

  expect(yaml).toContain('kind: Dataset')
  expect(yaml).toContain('name: invoices')
  expect(yaml).toContain('sources:\n    - ref: warehouse')
  expect(yaml).toContain('query: |\n    SELECT id, customer_id, issued_at, total\n    FROM invoices')
})

// A measure with no aggregate is refused on save, and every report using it
// would otherwise have to invent one.
test('a measure is given an aggregate', () => {
  const yaml = dataset({ name: 'x', slug: 'x', source: 'warehouse', fields, query: 'SELECT 1' })
  expect(yaml).toContain('aggregate: sum')
  // And a dimension is not.
  expect(yaml.match(/aggregate: sum/g)).toHaveLength(1)
})

// The difference between a dataset an end customer may read and one only the
// project may. It is a checkbox in the form and a predicate in the file.
test('row scope becomes a predicate', () => {
  const yaml = dataset({
    name: 'x', slug: 'x', source: 'warehouse', fields, query: 'SELECT 1',
    predicate: 'customer_id = {{ .scope.customer_id }}',
  })
  expect(yaml).toContain('rowLevelSecurity:')
  // Unquoted, the way every shipped example writes it: `{` is only special at
  // the start of a scalar, and this one begins with a column name.
  expect(yaml).toContain('predicate: customer_id = {{ .scope.customer_id }}')
})

test('a dataset with no scope has no predicate', () => {
  const yaml = dataset({ name: 'x', slug: 'x', source: 'warehouse', fields, query: 'SELECT 1' })
  expect(yaml).not.toContain('rowLevelSecurity')
})

// A definition is a file somebody commits. A secret in one is a secret in
// their git history for ever.
test('a datasource never carries the password', () => {
  const yaml = dataSource({
    name: 'Warehouse', slug: 'warehouse', kind: 'postgres',
    host: 'db.acme.example', port: 5432, database: 'analytics', user: 'reader',
  })
  expect(yaml).toContain('driver: postgres')
  expect(yaml).toContain('${secret:warehouse_password}')
  expect(yaml).not.toContain('password: ')
  expect(yaml).toContain('maxRows: 1000000')
})

test('an object store is addressed rather than connected to', () => {
  const yaml = dataSource({ name: 'Lake', slug: 'lake', kind: 'objectstore', uri: 's3://acme/events' })
  expect(yaml).toContain('driver: object-store')
  expect(yaml).toContain('uri: s3://acme/events')
  expect(yaml).toContain('format: parquet')
  expect(yaml).not.toContain('dsn:')
})

test('a report lays its blocks out under one interactive output', () => {
  const yaml = report({
    name: 'Billing', slug: 'billing', dataset: 'invoices', folder: 'Finance',
    blocks: [
      { kind: 'stat', title: 'Total billed', field: 'total', aggregate: 'sum' },
      { kind: 'chart', title: 'By month', field: 'total', groupBy: 'issued_at', grain: 'month' },
      { kind: 'table', title: 'Invoices', columns: ['issued_at', 'total'], pageSize: 50 },
    ],
  })

  expect(yaml).toContain('renderer: interactive')
  expect(yaml).toContain('label: Total billed')
  expect(yaml).toContain('value:\n            field: total\n            aggregate: sum')
  expect(yaml).toContain('chart: bar')
  expect(yaml).toContain('grain: month')
  expect(yaml).toContain('columns:\n            - issued_at\n            - total')
})

test('a schedule binds each row to a parameter', () => {
  const yaml = schedule({
    name: 'Monthly', slug: 'monthly-statements', report: 'statement', output: 'pdf',
    cron: '0 6 1 * *', timezone: 'Europe/Berlin',
    burstDataset: 'active-customers', recipientField: 'id',
    to: '{{ .row.email }}', subject: 'Your statement', retries: 3, alert: 'ops@acme.example',
  })

  expect(yaml).toContain('cron: "0 6 1 * *"')
  expect(yaml).toContain('over:\n      dataset: active-customers')
  expect(yaml).toContain('customer_id: "{{ .row.id }}"')
  expect(yaml).toContain('via: email')
  expect(yaml).toContain('retries: 3')
  expect(yaml).toContain('alert: ops@acme.example')
})

// A schedule that sends one document to one address is not a burst, and
// emitting an empty burst block would make it one over nothing.
test('a schedule with no burst has no burst block', () => {
  const yaml = schedule({
    name: 'Weekly', slug: 'weekly', report: 'summary', output: 'pdf',
    cron: '0 6 * * 1', timezone: 'UTC', to: 'ops@acme.example',
  })
  expect(yaml).not.toContain('burst:')
  expect(yaml).not.toContain('onFailure')
})

// The builder's palette lists things you can drop on a canvas; the format
// splits kind from chart type so a line chart is a new value rather than a new
// block kind. The same mismatch bit the embed component independently.
test("the builder's bar and line become charts", () => {
  const yaml = report({
    name: 'x', slug: 'x', dataset: 'invoices',
    blocks: [
      { kind: 'bar', title: 'Billed', field: 'total', groupBy: 'issued_at' },
      { kind: 'line', title: 'Trend', field: 'total', groupBy: 'issued_at' },
    ],
  })
  expect(yaml).toContain('kind: chart\n          title: Billed\n          chart: bar')
  expect(yaml).toContain('kind: chart\n          title: Trend\n          chart: line')
  expect(yaml).not.toContain('kind: bar')
  expect(yaml).not.toContain('kind: line')
})

/* Reading back. Each of these publishes what a form holds, reads the document
   the way the edit path does, and asserts nothing changed on the way through.
   That is the whole claim an edit path makes: opening a definition and saving
   it without touching anything leaves the definition alone. */

test('a datasource round trips', () => {
  const input = {
    name: 'Production warehouse', slug: 'warehouse', kind: 'postgres',
    host: 'db.internal', port: 5432, database: 'analytics', user: 'cronos',
  }
  const back = readDataSource(dataSource(input))
  expect(back.input).toMatchObject(input)
  expect(back.drops).toEqual([])
})

test('a dataset round trips, row scope included', () => {
  const input = {
    name: 'Invoices', slug: 'invoices', description: 'One row per invoice.',
    source: 'warehouse', query: 'SELECT id, total\nFROM invoices\n',
    fields: [
      { name: 'id', type: 'string', role: 'dimension', label: 'ID', hidden: false, format: 'preformatted' },
      { name: 'total', type: 'number', role: 'measure', label: '', hidden: false, format: 'currency' },
    ] as Field[],
    predicate: 'customer_id = {{ .scope.customer_id }}',
  }
  const back = readDataset(dataset(input))
  expect(back.input).toEqual(input)
  expect(back.drops).toEqual([])
})

test('a report round trips, and blocks come back as palette entries', () => {
  const input = {
    name: 'Monthly statement', slug: 'monthly-statement', folder: 'Finance',
    dataset: 'invoices',
    blocks: [
      { kind: 'stat', title: 'Billed', field: 'total', aggregate: 'sum' },
      { kind: 'line', title: 'Over time', field: 'total', aggregate: 'sum', groupBy: 'issued_at', grain: 'month', chart: 'line' },
      { kind: 'table', title: 'Detail', columns: ['id', 'total'], pageSize: 25 },
    ],
  }
  const back = readReport(report(input))
  expect(back.input.blocks.map((b) => b.kind)).toEqual(['stat', 'line', 'table'])
  expect(back.drops).toEqual([])
})

test('a schedule round trips, burst binding included', () => {
  const input = {
    name: 'Monthly invoices', slug: 'monthly-invoices', report: 'statement',
    output: 'pdf', cron: '0 6 1 * *', timezone: 'Europe/London',
    burstDataset: 'customers', recipientField: 'customer_id', concurrency: 4,
    to: '{{ .row.email }}', subject: 'Your statement', channel: 'email',
    retries: 3, alert: 'ops@example.com',
  }
  const back = readSchedule(schedule(input))
  expect(back.input).toMatchObject(input)
  expect(back.drops).toEqual([])
})

/* The point of the drop list, and its limit. The shared filter survives: it is
   a spec key the form never writes, so a save folds it back untouched. The
   second output profile does not: outputs is a list the builder rewrites
   wholesale, and grafting a stored entry back into a rewritten list would
   attach it to whatever now sits at that index. */
test('what a save cannot keep is named, and what it can is kept', () => {
  const stored = [
    'apiVersion: cronos.dev/v1',
    'kind: Report',
    'metadata:',
    '  name: statement',
    'spec:',
    '  dataset: invoices',
    '  filters:',
    '    - name: region',
    '      type: enum',
    '  outputs:',
    '    - name: interactive',
    '      renderer: interactive',
    '      layout:',
    '        - kind: table',
    '          columns: [id]',
    '    - name: pdf',
    '      renderer: paginated',
    '      layout: []',
    '',
  ].join('\n')

  const back = readReport(stored)
  expect(back.input.dataset).toBe('invoices')
  expect(back.drops).toEqual(['spec.outputs[1]'])
  // Carried rather than merely warned about: the saved document still has it.
  expect(withCarry(report(back.input), back)).toContain('name: region')
})

/* The last two things a stored report could hold that the builder could not
   show. Both live inside outputs[].layout[], which the builder rewrites
   wholesale — so carry-over cannot reach them and they had to be read back. */

test('a block filter and a sort survive the round trip', () => {
  const input = {
    name: 'Billing', slug: 'billing', dataset: 'invoices',
    blocks: [
      { kind: 'stat', title: 'Outstanding', field: 'total', aggregate: 'sum',
        filter: "status = 'overdue'" },
      { kind: 'table', title: 'Detail', columns: ['id', 'total'],
        sort: [{ field: 'issued_at', dir: 'desc' }, { field: 'total' }] },
    ],
  }
  const back = readReport(report(input))

  expect(back.input.blocks[0]?.filter).toBe("status = 'overdue'")
  expect(back.input.blocks[1]?.sort).toEqual([
    { field: 'issued_at', dir: 'desc' },
    // Ascending is the absence of a direction, in the file and back.
    { field: 'total', dir: undefined },
  ])
  expect(back.drops).toEqual([])
})

test('a block with neither writes neither', () => {
  const emitted = report({
    name: 'Plain', slug: 'plain', dataset: 'invoices',
    blocks: [{ kind: 'stat', title: 'Billed', field: 'total' }],
  })
  expect(emitted).not.toContain('filter')
  expect(emitted).not.toContain('sort')
})
