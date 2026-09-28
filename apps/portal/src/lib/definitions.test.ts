import { expect, test } from 'bun:test'
import {
  connectionFor, dataset, dataSource, passwordSecret, readDataset, readDataSource,
  readReport, readSchedule, report, schedule, withCarry, type SourceInput,
} from './definitions'
import type { Field, Param } from './types'
import { relayer } from './maps'

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
  expect(yaml).toContain('${secret:warehouse_password|url}')
  expect(yaml).not.toContain('password: ')
  expect(yaml).toContain('maxRows: 1000000')
})

/*
 * The name the file reads and the name the wizard stores the password under
 * are one function now. They were one line apart and never connected: the file
 * named `<slug>_password`, the form threw the password away, and every
 * database connected through the portal named a secret nothing held.
 */
test('a new source stores its password under the name its connection string reads', () => {
  const input = { name: 'Scratch', slug: 'scratch', kind: 'postgres', host: 'db', user: 'reader' }
  expect(passwordSecret(input)).toBe('scratch_password')
  expect(dataSource(input)).toContain('reader:${secret:scratch_password|url}@db')
})

/*
 * The password is stored as typed, and a connection string that reads it inside
 * a URL says so: `|url`, which the server encodes as it substitutes.
 *
 * It was stored percent-encoded by this form instead, which worked for a
 * password typed here and broke for the same secret replaced from Settings,
 * where the form could not know a URL would read it. A space came back from
 * the probe as "unexpected spaces found in password"; an @ would have ended the
 * userinfo early and connected somewhere else.
 */
test('a new source reads its password encoded for the URL it sits in', () => {
  const yaml = dataSource({ name: 'Scratch', slug: 'scratch', kind: 'postgres', host: 'db', port: 5432,
    database: 'x', user: 'reader' })
  expect(yaml).toContain('postgres://reader:${secret:scratch_password|url}@db:5432/x')
})

/*
 * Each driver's own grammar. This wrote one URL for all of them, and MySQL's
 * driver cannot parse a URL at all — it reads `user:password@tcp(host:port)/db`
 * — while SQL Server's takes a URL's path for the instance name, so the
 * database went where no database is read. A source of either kind connected
 * through the wizard never connected.
 */
test('a connection string is written in the form its driver reads', () => {
  const my = dataSource({ name: 'ERP', slug: 'erp', kind: 'mysql', host: 'db', port: 3306,
    database: 'erp', user: 'root' })
  // As written: this driver takes the password raw, so there is nothing to encode.
  expect(my).toContain('root:${secret:erp_password}@tcp(db:3306)/erp?parseTime=true')
  const ms = dataSource({ name: 'ERP', slug: 'erp', kind: 'sqlserver', host: 'db', port: 1433,
    database: 'erp', user: 'sa' })
  expect(ms).toContain('sqlserver://sa:${secret:erp_password|url}@db:1433?database=erp')
})

test('each form reads back into its parts and writes back the same', () => {
  for (const [driver, dsn] of [
    ['mysql', 'root:${secret:erp_password}@tcp(db:3306)/erp?parseTime=true'],
    ['sqlserver', 'sqlserver://sa:${secret:erp_password|url}@db:1433?database=erp&encrypt=disable'],
    ['postgres', 'postgres://reader:${secret:wh|url}@db:5432/analytics?sslmode=require'],
  ] as const) {
    const read = readDataSource([
      'apiVersion: cronos.dev/v1', 'kind: DataSource', 'metadata:', '  name: erp', 'spec:',
      `  driver: ${driver}`, `  dsn: "${dsn}"`,
    ].join('\n')).input
    expect(read.host).toBe('db')
    expect(read.passwordSecret).toBeDefined()
    expect(dataSource({ ...read, dsn: undefined })).toContain(dsn)
  }
})

/* A file somebody wrote by hand names its own secret. Rebuilding the string
   from its parts — a new host, say — must keep naming that one, or the edit
   quietly points the source at a secret nothing has stored. */
test('an edited source keeps reading the secret its file names', () => {
  const stored = readDataSource([
    'apiVersion: cronos.dev/v1', 'kind: DataSource', 'metadata:', '  name: warehouse', 'spec:',
    '  driver: postgres',
    '  dsn: "postgres://reader:${secret:warehouse-password}@db.internal:5432/analytics?sslmode=require"',
  ].join('\n'))
  expect(stored.input.passwordSecret).toBe('warehouse-password')
  expect(passwordSecret(stored.input)).toBe('warehouse-password')

  const moved = dataSource({ ...stored.input, dsn: undefined, host: 'db.new' })
  expect(moved).toContain('reader:${secret:warehouse-password}@db.new:5432/analytics')
})

/* No secret in the password's place — a literal, or nothing — is not a name to
   store under. The form then stores it under the source's own name and
   rebuilds the string to read that. */
test('a connection string with no reference names no password secret', () => {
  expect(passwordNamedBy('postgres://reader:hunter2@db/x')).toBeUndefined()
  expect(passwordNamedBy('postgres://reader@db/x')).toBeUndefined()
  expect(passwordNamedBy('file:cronos-demo?mode=memory&cache=shared')).toBeUndefined()
  expect(passwordNamedBy('postgres://reader:pre${secret:x}@db/x')).toBeUndefined()
})

/*
 * An edited host is written.
 *
 * The form seeds its connection-string field with the stored string for every
 * kind of source — the SQLite screen shows it — and chose `value.dsn || …`, so
 * for a database the stored string always won: a source moved to a new host
 * went on connecting to the old one, with nothing on screen to say so.
 */
const handWritten = () => readDataSource([
  'apiVersion: cronos.dev/v1', 'kind: DataSource', 'metadata:', '  name: warehouse', 'spec:',
  '  driver: postgres',
  '  dsn: "postgres://reader:${secret:warehouse-password}@db.internal:5432/analytics?sslmode=require"',
].join('\n')).input

const asLoaded = (input: SourceInput) => ({
  dsn: input.dsn ?? '', host: input.host ?? '', port: input.port ?? 5432,
  database: input.database ?? '', user: input.user ?? '', password: '',
})

test('an edited host is written, and the stored string’s options survive it', () => {
  const stored = handWritten()
  const form = { ...asLoaded(stored), host: 'db.new' }
  const chosen = connectionFor('sql', form, stored)
  expect(chosen).toBeUndefined()

  const yaml = dataSource({ ...stored, host: form.host, dsn: chosen })
  expect(yaml).toContain('reader:${secret:warehouse-password}@db.new:5432/analytics?sslmode=require')
})

test('an untouched connection keeps its string exactly as written', () => {
  const stored = handWritten()
  expect(connectionFor('sql', asLoaded(stored), stored)).toBe(stored.dsn)
  // A new password where the string already names a secret is stored under
  // that name, and changes nothing about the string.
  expect(connectionFor('sql', { ...asLoaded(stored), password: 'x' }, stored)).toBe(stored.dsn)
})

/* Where the string names no secret, a typed password has to be read from
   somewhere, so the string is rebuilt to name one. */
test('a literal password is replaced by a reference when a new one is typed', () => {
  const stored = readDataSource([
    'apiVersion: cronos.dev/v1', 'kind: DataSource', 'metadata:', '  name: warehouse', 'spec:',
    '  driver: postgres', '  dsn: "postgres://reader:hunter2@db.internal:5432/analytics"',
  ].join('\n')).input
  expect(connectionFor('sql', asLoaded(stored), stored)).toBe(stored.dsn)
  expect(connectionFor('sql', { ...asLoaded(stored), password: 'x' }, stored)).toBeUndefined()
})

test('a connection string typed whole is written as typed, and a new source keeps nothing', () => {
  const typed = { ...asLoaded(handWritten()), dsn: 'file:/var/lib/cronos/warehouse.db' }
  expect(connectionFor('dsn', typed, handWritten())).toBe('file:/var/lib/cronos/warehouse.db')
  expect(connectionFor('sql', { ...typed, dsn: '' }, undefined)).toBeUndefined()
})

/** The password secret a stored source with this connection string reads. */
function passwordNamedBy(dsn: string): string | undefined {
  return readDataSource([
    'apiVersion: cronos.dev/v1', 'kind: DataSource', 'metadata:', '  name: w', 'spec:',
    '  driver: postgres', `  dsn: "${dsn}"`,
  ].join('\n')).input.passwordSecret
}

test('an object store is addressed rather than connected to', () => {
  const yaml = dataSource({ name: 'Lake', slug: 'lake', kind: 'objectstore', uri: 's3://acme/events' })
  expect(yaml).toContain('driver: object-store')
  expect(yaml).toContain('uri: s3://acme/events')
  expect(yaml).toContain('format: parquet')
  expect(yaml).not.toContain('dsn:')
})

/* A private bucket needs a key, and the form had nowhere to put one — so the
   only way to define a readable lake was to write the YAML by hand, which is
   where somebody pastes the key instead of a reference to it. */
test('a lake carries how it is reached', () => {
  const yaml = dataSource({
    name: 'Lake', slug: 'lake', kind: 'objectstore', uri: 's3://acme/events',
    region: 'eu-central-1', storeEndpoint: 'http://minio.internal:9000',
    credentials: '${secret:lake_creds}',
  })
  expect(yaml).toContain('region: eu-central-1')
  expect(yaml).toContain('endpoint: http://minio.internal:9000')
  expect(yaml).toContain('credentials: ${secret:lake_creds}')
})

/* Blank is not the same as absent. A region written out empty is a key in the
   file that means nothing and reads as though it did. */
test('a public bucket writes no key it does not have', () => {
  const yaml = dataSource({ name: 'Open', slug: 'open', kind: 'objectstore', uri: 's3://open/data' })
  expect(yaml).not.toContain('region:')
  expect(yaml).not.toContain('endpoint:')
  expect(yaml).not.toContain('credentials:')
})

/* Unlike a password, which the server never returns. What the file holds is
   the reference, so reopening shows it and saving keeps it — blanking it on an
   edit would quietly drop the credential from a working source. */
test('a lake round trips how it is reached', () => {
  const input = {
    name: 'Lake', slug: 'lake', kind: 'objectstore', uri: 's3://acme/events',
    region: 'eu-central-1', storeEndpoint: 'http://minio.internal:9000',
    credentials: '${secret:lake_creds}',
  }
  const back = readDataSource(dataSource(input))
  expect(back.input).toMatchObject(input)
  expect(back.drops).toEqual([])
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

/* Parameters were carried through an edit before they were modelled, so a
   parameterised dataset survived being opened and saved — and could only be
   created by editing the file, which made the portal a second-class way to
   author exactly the datasets that need the most care. */

test('parameters round trip, including what only an enum may have', () => {
  const input = {
    name: 'Invoices', slug: 'invoices', source: 'warehouse',
    query: 'SELECT id FROM invoices WHERE status = {{ .params.status }}\n',
    fields: [{ name: 'id', type: 'string', role: 'dimension', label: '', hidden: false, format: 'preformatted' }] as Field[],
    params: [
      { name: 'from', type: 'date', label: 'From', required: true, multiple: false, values: [], default: '2020-01-01' },
      { name: 'status', type: 'enum', label: 'Status', required: false, multiple: true,
        values: ['paid', 'overdue'], default: undefined },
    ] as Param[],
  }

  const back = readDataset(dataset(input))
  expect(back.input.params).toEqual(input.params)
  expect(back.drops).toEqual([])
})

// Values on anything but an enum is a constraint the engine does not apply,
// which reads as one that does.
test('permitted values are written only for an enum', () => {
  const emitted = dataset({
    name: 'X', slug: 'x', source: 'warehouse', query: 'SELECT 1\n', fields: [],
    params: [{ name: 'note', type: 'string', values: ['a', 'b'] }],
  })
  expect(emitted).not.toContain('values')
})

// A dataset with no parameters writes none, rather than an empty list that
// reads as a decision somebody made.
test('a dataset with no parameters says nothing about them', () => {
  const emitted = dataset({
    name: 'X', slug: 'x', source: 'warehouse', query: 'SELECT 1\n', fields: [], params: [],
  })
  expect(emitted).not.toContain('params')
})

/*
 * The chart types past bar and line.
 *
 * The round trip is the part that breaks silently: a block written correctly
 * and read back as something else looks like the author changed it, and the
 * builder is where they will next hit save.
 */
test('a stacked chart carries its series and its stacking', () => {
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'invoices',
    blocks: [{
      kind: 'bar', title: 'By month', field: 'total', aggregate: 'sum',
      groupBy: 'issued_at', grain: 'month', series: 'status', stacked: true,
    }],
  })
  expect(yaml).toContain('series:\n            field: status')
  expect(yaml).toContain('stacked: true')

  const loaded = readReport(yaml)
  // Nothing written was left unread: a drop here is a field the builder would
  // silently delete the next time somebody saved.
  expect(loaded.drops).toEqual([])
  const back = loaded.input.blocks[0]
  expect(back?.kind).toBe('bar')
  expect(back?.series).toBe('status')
  expect(back?.stacked).toBe(true)
})

test('a plot writes its horizontal measure as xValue, not as x', () => {
  // x is a dimension everywhere else in the format — what each dot *is*. One
  // field cannot be both that and a number, and letting it try is how a date
  // grain ends up on a measure.
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'invoices',
    blocks: [{
      kind: 'bubble', title: 'Paid against billed', groupBy: 'customer_name',
      xField: 'paid', field: 'total', sizeField: 'count', aggregate: 'sum',
      // A grain the builder left behind from another kind must not survive.
      grain: 'month',
    }],
  })
  expect(yaml).toContain('chart: bubble')
  expect(yaml).toContain('xValue:\n            field: paid')
  expect(yaml).not.toContain('grain: month')

  const loaded = readReport(yaml)
  expect(loaded.drops).toEqual([])
  const back = loaded.input.blocks[0]
  expect(back?.kind).toBe('bubble')
  expect(back?.xField).toBe('paid')
  expect(back?.sizeField).toBe('count')
})

test('a map carries its layers and geography', () => {
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'depots',
    blocks: [{
      kind: 'map', title: 'Depots', groupBy: 'region', field: 'parcels', aggregate: 'sum',
      map: { layers: ['polygon', 'scatter'], geometry: 'shape', lat: 'lat', lon: 'lon' },
    }],
  })
  expect(yaml).toContain('chart: map')
  expect(yaml).toContain('geometry: shape')

  const loaded = readReport(yaml)
  expect(loaded.drops).toEqual([])
  const back = loaded.input.blocks[0]
  expect(back?.kind).toBe('map')
  expect(back?.map?.layers).toEqual(['polygon', 'scatter'])
  expect(back?.map?.lat).toBe('lat')
})

test('a basemap is written only with the credit line that has to go with it', () => {
  // The server refuses a tile template with no attribution, because every tile
  // source requires one be displayed. Emitting half of it here would turn that
  // into a save that fails with nothing on screen having asked for it.
  const withCredit = report({
    name: 'R', slug: 'r', dataset: 'depots',
    blocks: [{
      kind: 'map', title: 'Depots', groupBy: 'region', field: 'parcels',
      map: {
        layers: ['scatter'], lat: 'lat', lon: 'lon',
        basemap: 'https://tile.example.org/{z}/{x}/{y}.png',
        attribution: '© OpenStreetMap contributors',
      },
    }],
  })
  expect(withCredit).toContain('url: https://tile.example.org/{z}/{x}/{y}.png')
  expect(withCredit).toContain('attribution: © OpenStreetMap contributors')

  const none = report({
    name: 'R', slug: 'r', dataset: 'depots',
    blocks: [{
      kind: 'map', title: 'Depots', groupBy: 'region', field: 'parcels',
      map: { layers: ['scatter'], lat: 'lat', lon: 'lon' },
    }],
  })
  expect(none).not.toContain('basemap')
})

test('a map block is the only kind that writes a map', () => {
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'invoices',
    blocks: [{
      kind: 'bar', title: 'Billed', field: 'total', groupBy: 'status',
      // Left over from a block that was a map a moment ago. The union is flat,
      // so only the kind decides which fields are written.
      map: { layers: ['scatter'], lat: 'lat', lon: 'lon' },
    }],
  })
  expect(yaml).not.toContain('lat')
})

/*
 * What a map holds that the builder has no control for.
 *
 * A map sits inside outputs[].layout[], which the builder rewrites wholesale,
 * so carry-over cannot reach it: a key the reader skips is a key the next save
 * deletes. `region` and `simplify` went that way for as long as the builder
 * could open a map, and a basemap's key would have moved the report onto the
 * deployment's account without anybody choosing to.
 */

/** A stored report holding one map, whose `map:` is the lines given. */
function storedMap(...lines: string[]): string {
  return [
    'apiVersion: cronos.dev/v1',
    'kind: Report',
    'metadata: {name: r, title: R}',
    'spec:',
    '  dataset: drops',
    '  outputs:',
    '    - name: interactive',
    '      renderer: interactive',
    '      layout:',
    '        - kind: chart',
    '          chart: map',
    '          title: Drops',
    '          x: {field: drop_id}',
    '          y: {field: parcels, aggregate: sum}',
    '          map:',
    ...lines.map((l) => `            ${l}`),
    '',
  ].join('\n')
}

test('a provider basemap keeps what the builder never shows', () => {
  const back = readReport(storedMap(
    'layers: [scatter]',
    'lat: lat',
    'lon: lon',
    'basemap:',
    '  provider: google',
    '  style: satellite',
    '  key: ${secret:google-maps-acme}',
    '  language: id',
    '  region: ID',
    '  maxZoom: 16',
  ))
  // Measured against what a save writes, so empty means every key survives.
  expect(back.drops).toEqual([])
  expect(back.input.blocks[0]?.map).toMatchObject({
    provider: 'google', style: 'satellite', key: '${secret:google-maps-acme}',
    language: 'id', basemapRegion: 'ID', maxZoom: 16,
  })

  const resaved = report(back.input)
  expect(resaved).toContain('key: ${secret:google-maps-acme}')
  expect(resaved).toContain('region: ID')
  expect(resaved).toContain('maxZoom: 16')
  // The provider's credit line is its terms' wording, and the server writes it.
  expect(resaved).not.toContain('url:')
  expect(resaved).not.toContain('attribution')
  expect(readReport(resaved).input.blocks).toEqual(back.input.blocks)
})

test('a Mapbox Studio style survives the round trip', () => {
  const back = readReport(storedMap(
    'lat: lat',
    'lon: lon',
    'basemap:',
    '  provider: mapbox',
    '  style: acme/cjx1abc2de3',
    '  key: ${secret:mapbox-acme}',
  ))
  expect(back.drops).toEqual([])
  expect(back.input.blocks[0]?.map?.style).toBe('acme/cjx1abc2de3')
  expect(report(back.input)).toContain('style: acme/cjx1abc2de3')
})

test('a label field, a simplification and a hexagon width survive the round trip', () => {
  const shapes = readReport(storedMap(
    'layers: [polygon]',
    'geometry: zone_shape',
    'region: zone_name',
    'simplify: 0.00001',
  ))
  expect(shapes.drops).toEqual([])
  expect(shapes.input.blocks[0]?.map).toMatchObject({ region: 'zone_name', simplify: 0.00001 })

  const hexes = readReport(storedMap('layers: [hexbin]', 'lat: lat', 'lon: lon', 'hexKm: 5'))
  expect(hexes.drops).toEqual([])
  expect(hexes.input.blocks[0]?.map?.hexKm).toBe(5)
  expect(report(hexes.input)).toContain('hexKm: 5')
})

// Negative keeps every vertex. It is not zero, and a writer that treated it as
// "not set" would put the default simplification back on somebody's coastline.
test('a simplification that keeps every vertex is written', () => {
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'zones',
    blocks: [{
      kind: 'map', title: 'Zones', groupBy: 'zone', field: 'parcels',
      map: { layers: ['polygon'], geometry: 'shape', simplify: -1 },
    }],
  })
  expect(yaml).toContain('simplify: -1')
})

test('a custom tile basemap round trips as before', () => {
  const back = readReport(storedMap(
    'layers: [scatter]',
    'lat: lat',
    'lon: lon',
    'basemap:',
    '  url: https://tile.example.org/{z}/{x}/{y}.png',
    '  attribution: © Example contributors',
    '  maxZoom: 18',
  ))
  expect(back.drops).toEqual([])
  expect(back.input.blocks[0]?.map).toMatchObject({
    basemap: 'https://tile.example.org/{z}/{x}/{y}.png',
    attribution: '© Example contributors', maxZoom: 18,
  })
  expect(back.input.blocks[0]?.map?.provider).toBeUndefined()
  expect(report(back.input)).not.toContain('provider')
})

/* The server refuses a basemap that names a provider and a url, and a
   provider with somebody else's credit line. The inspector clears one form
   when the other is picked; this is what holds if something skips it. */
test('a basemap is written in one form, never both', () => {
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'depots',
    blocks: [{
      kind: 'map', title: 'Depots', groupBy: 'region', field: 'parcels',
      map: {
        layers: ['scatter'], lat: 'lat', lon: 'lon', provider: 'mapbox', style: 'dark',
        basemap: 'https://tile.example.org/{z}/{x}/{y}.png', attribution: '© Example',
      },
    }],
  })
  expect(yaml).toContain('provider: mapbox')
  expect(yaml).not.toContain('url:')
  expect(yaml).not.toContain('attribution')

  // A url still being typed is no basemap yet.
  const typing = report({
    name: 'R', slug: 'r', dataset: 'depots',
    blocks: [{
      kind: 'map', title: 'Depots', groupBy: 'region', field: 'parcels',
      map: { layers: ['scatter'], lat: 'lat', lon: 'lon', basemap: '', attribution: '© Ex' },
    }],
  })
  expect(typing).not.toContain('basemap')
})

test('a map colours its points by the series it is given', () => {
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'drops',
    blocks: [{
      kind: 'map', title: 'Drops by carrier', groupBy: 'drop_id', field: 'parcels',
      series: 'carrier',
      map: { layers: ['cluster', 'scatter'], lat: 'lat', lon: 'lon' },
    }],
  })
  expect(yaml).toContain('series:\n            field: carrier')

  const loaded = readReport(yaml)
  expect(loaded.drops).toEqual([])
  expect(loaded.input.blocks[0]?.series).toBe('carrier')
  expect(loaded.input.blocks[0]?.map?.layers).toEqual(['cluster', 'scatter'])
})

test('a combo writes its measures and how each is drawn', () => {
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'invoices',
    blocks: [{
      kind: 'combo', title: 'Billed and margin', groupBy: 'issued_at', grain: 'month',
      metrics: [
        { field: 'total', aggregate: 'sum', label: 'Billed', draw: 'bar' },
        { field: 'margin', aggregate: 'avg', label: 'Margin', draw: 'line', secondary: true },
      ],
    }],
  })
  expect(yaml).toContain('chart: combo')
  expect(yaml).toContain('draw: line')
  // Per measure and never a flag on the chart — two scales on one plot is the
  // most-flagged mistake in charting, so it is reachable and never accidental.
  expect(yaml).toContain('axis: secondary')
  // Metrics replace y rather than joining it.
  expect(yaml).not.toContain('\n          y:')

  const loaded = readReport(yaml)
  expect(loaded.drops).toEqual([])
  const back = loaded.input.blocks[0]
  expect(back?.kind).toBe('combo')
  expect(back?.metrics?.[1]?.secondary).toBe(true)
  expect(back?.metrics?.[1]?.draw).toBe('line')
})

test('a funnel of columns has no x to bucket by', () => {
  // Its stages are the measures, in the order they are listed. The server
  // refuses an x here, so writing one would be a save that fails.
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'invoices',
    blocks: [{
      kind: 'funnel', title: 'Conversion',
      // Left over from another kind the block used to be.
      groupBy: 'status',
      metrics: [
        { field: 'quoted', label: 'Quoted' },
        { field: 'total', label: 'Paid' },
      ],
    }],
  })
  expect(yaml).toContain('chart: funnel')
  expect(yaml).not.toContain('x:')

  const back = readReport(yaml).input.blocks[0]
  expect(back?.metrics?.map((m) => m.label)).toEqual(['Quoted', 'Paid'])
})

test('a gauge writes one kind of target, never both', () => {
  const fixed = report({
    name: 'R', slug: 'r', dataset: 'invoices',
    blocks: [{
      kind: 'gauge', title: 'Against plan', field: 'total', aggregate: 'sum',
      target: { value: 100000, label: 'Plan' },
    }],
  })
  expect(fixed).toContain('value: 100000')
  expect(fixed).not.toContain('target:\n            field')

  const measured = report({
    name: 'R', slug: 'r', dataset: 'invoices',
    blocks: [{
      kind: 'gauge', title: 'Against quota', field: 'total',
      target: { field: 'quota', aggregate: 'sum' },
    }],
  })
  expect(measured).toContain('field: quota')

  const back = readReport(fixed).input.blocks[0]
  expect(back?.target?.value).toBe(100000)
  expect(back?.target?.field).toBeUndefined()
})

test('metrics are written only by the kinds that read them', () => {
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'invoices',
    blocks: [{
      kind: 'bar', title: 'Billed', field: 'total', groupBy: 'status',
      // Left behind by a block that was a combo a moment ago. The union is
      // flat, so only the kind decides what is written.
      metrics: [{ field: 'margin', draw: 'line' }],
      target: { value: 10 },
    }],
  })
  expect(yaml).not.toContain('metrics')
  expect(yaml).not.toContain('target')
})

/*
 * Report filters.
 *
 * These were neither written nor read for as long as the builder existed, and
 * `drops` did not cover them — so opening a report that had filters and saving
 * it deleted every one of them, silently, and they are the whole interactive
 * surface of the report.
 */
test('a report keeps its filters when it is opened and saved again', () => {
  const stored = `apiVersion: cronos.dev/v1
kind: Report
metadata: {name: r, title: R}
spec:
  dataset: invoices
  filters:
    - name: period
      label: Period
      type: date
      bind: {invoices: issued_at, shipments: dispatched_at}
    - name: status
      label: Status
      type: enum
      values: [sent, overdue, paid]
      bind: {invoices: status}
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: stat
          label: Billed
          value: {field: total, aggregate: sum}
`
  const back = readReport(stored)
  expect(back.drops).toEqual([])
  expect(back.input.filters?.map((f) => f.name)).toEqual(['period', 'status'])

  const resaved = report(back.input)
  expect(resaved).toContain('name: period')
  // The bind map is a field per dataset, because a report's blocks may read
  // different ones and guessing is how a filter applies to half a screen.
  expect(resaved).toContain('invoices: issued_at')
  expect(resaved).toContain('shipments: dispatched_at')
  expect(resaved).toContain('values:')
})

test('values are written only on the type that takes them', () => {
  // The server refuses a non-enum filter that lists values, so writing them
  // would be a save that fails for a reason the form never showed.
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'invoices',
    filters: [{ name: 'period', type: 'date', values: ['left', 'over'], bind: { invoices: 'issued_at' } }],
    blocks: [{ kind: 'stat', title: 'Billed', field: 'total' }],
  })
  expect(yaml).toContain('name: period')
  expect(yaml).not.toContain('left')
})

test('a half-finished filter is written out, not quietly dropped', () => {
  // The server refuses a filter bound to nothing — it is a control that does
  // nothing, shown to somebody who will reasonably expect it to work — and it
  // says so in a sentence naming what is missing. Deleting the author's
  // half-finished filter on save and saying nothing is the alternative.
  const yaml = report({
    name: 'R', slug: 'r', dataset: 'invoices',
    filters: [{ name: 'half', type: 'string', bind: {} }],
    blocks: [{ kind: 'stat', title: 'Billed', field: 'total' }],
  })
  expect(yaml).toContain('name: half')
})

/*
 * What a map, a filter and the filter bar carry that the builder only keeps.
 *
 * A map's overlays sit inside outputs[].layout[], which the builder rewrites
 * wholesale, and a filter's control inside spec.filters[], which it rewrites
 * too: neither reaches carry-over, so a key not read here is deleted by the
 * next save.
 */
test('a map keeps the datasets drawn over it', () => {
  const back = readReport(storedMap(
    'layers: [heat]',
    'lat: lat',
    'lon: lon',
    'overlays:',
    '  - dataset: depots',
    '    title: Depots',
    '    x: {field: name}',
    '    y: {field: capacity, aggregate: sum}',
    '    map:',
    '      layers: [scatter]',
    '      lat: lat',
    '      lon: lon',
  ))
  expect(back.drops).toEqual([])
  const resaved = report(back.input)
  expect(resaved).toContain('overlays:')
  expect(resaved).toContain('dataset: depots')
  expect(readReport(resaved).input.blocks).toEqual(back.input.blocks)
})

test('a filter keeps the control its author chose', () => {
  const stored = `apiVersion: cronos.dev/v1
kind: Report
metadata: {name: r, title: R}
spec:
  dataset: drops
  filters:
    - name: depot
      type: enum
      values: [North, South]
      control: checkboxes
      bind: {drops: depot}
    - name: where
      type: area
      bind: {drops: "lat,lon"}
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: stat
          label: Parcels
          value: {field: parcels, aggregate: sum}
`
  const back = readReport(stored)
  expect(back.drops).toEqual([])
  expect(back.input.filters?.[0]?.control).toBe('checkboxes')
  // An area narrows by a pair of fields, and the pair is what goes back.
  expect(back.input.filters?.[1]?.bind).toEqual({ drops: 'lat,lon' })
  const resaved = report(back.input)
  expect(resaved).toContain('control: checkboxes')
  expect(resaved).toContain('drops: lat,lon')
})

// How a map shades is read and written whole: a builder save that dropped a
// diverging ramp would repaint a map of changes in one hue, and nothing on
// screen would say the colours had stopped meaning what they did.
test('a map keeps how its shades are classed and coloured', () => {
  const back = readReport(storedMap(
    'layers: [polygon]',
    'geometry: shape',
    'classify: custom',
    'breaks: [90, 95, 98]',
    'ramp: diverging',
    'midpoint: 95',
  ))
  expect(back.drops).toEqual([])
  expect(back.input.blocks[0]?.map).toMatchObject({
    classify: 'custom', breaks: [90, 95, 98], ramp: 'diverging', midpoint: 95,
  })
  const resaved = report(back.input)
  expect(resaved).toContain('classify: custom')
  expect(resaved).toContain('ramp: diverging')
  expect(resaved).toContain('midpoint: 95')
  expect(readReport(resaved).input.blocks).toEqual(back.input.blocks)
})

// Moved off the layers that are shaded, how the shades were classed goes with
// them: the server refuses it on a map with nothing to shade.
test('a map with nothing shaded keeps no classes', () => {
  const moved = relayer({
    map: { layers: ['polygon'], geometry: 'shape', classify: 'jenks', ramp: 'diverging', midpoint: 5 },
  }, ['scatter'])
  expect(moved.map?.classify).toBeUndefined()
  expect(moved.map?.ramp).toBeUndefined()
  expect(moved.map?.midpoint).toBeUndefined()
})

// What a map adds to its marks is read and written whole, for the same reason
// its shades are: a save that dropped a radius would refuse to store, and one
// that dropped labels would quietly unname a map.
test('a map keeps its radius, its names and its moving flows', () => {
  const back = readReport(storedMap(
    'layers: [radius, scatter, flow]',
    'lat: lat',
    'lon: lon',
    'toLat: to_lat',
    'toLon: to_lon',
    'radiusKm: 25',
    'labels: true',
    'animate: true',
  ))
  expect(back.drops).toEqual([])
  expect(back.input.blocks[0]?.map).toMatchObject({ radiusKm: 25, labels: true, animate: true })
  expect(readReport(report(back.input)).input.blocks).toEqual(back.input.blocks)
})

// An h3 layer's cells round-trip whole: which column, and how fine.
test('a map keeps its H3 cells', () => {
  const back = readReport(storedMap('layers: [h3]', 'h3: cell', 'h3Resolution: 6'))
  expect(back.drops).toEqual([])
  expect(back.input.blocks[0]?.map).toMatchObject({ h3: 'cell', h3Resolution: 6 })
  expect(readReport(report(back.input)).input.blocks).toEqual(back.input.blocks)
})
