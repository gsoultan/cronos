/**
 * The source kinds, and how much of a filter each can absorb.
 *
 * Pushdown is surfaced in the UI rather than hidden, because it is the
 * difference between a 200ms tile and a 40-second one — and the person
 * choosing the source is the only one who can still change the answer.
 * See AGENTS.md § Two planes.
 */

export type SourceKind =
  | 'postgres' | 'mysql' | 'sqlserver' | 'sqlite' | 'clickhouse' | 'bigquery'
  | 'objectstore' | 'api' | 'excel'

/*
 * How a source is connected to, which decides what the form asks for.
 *
 * `dsn` is one field: the connection string, as written. It exists because
 * `sql` asks for a host, a port, a database and a user, and there are sources
 * that have none of those — SQLite is a path, and a driver this build has no
 * screen for is whatever its own documentation says. Asking for a port to
 * connect to a file is not a smaller mistake than asking for nothing.
 */
export type Shape = 'sql' | 'dsn' | 'object' | 'api' | 'file'
export type Pushdown = 'full' | 'partial' | 'none' | 'declared'

export interface SourceSpec {
  id: SourceKind
  label: string
  hint: string
  icon: string
  shape: Shape
  connectHint: string
  pushdown: Pushdown
  pushdownLabel: string
  pushdownHint: string
}

const FULL = {
  pushdown: 'full' as const,
  pushdownLabel: 'Filters run in the database',
  pushdownHint: 'Filters compile to SQL, so only matching rows ever leave the source.',
}

export const SOURCE_KINDS: SourceSpec[] = [
  {
    id: 'postgres', label: 'PostgreSQL', hint: 'Most common', icon: '🐘', shape: 'sql',
    connectHint: 'A read-only user is enough — cronos never writes to your database.',
    ...FULL,
  },
  {
    id: 'mysql', label: 'MySQL', hint: 'Or MariaDB', icon: '🐬', shape: 'sql',
    connectHint: 'A read-only user is enough — cronos never writes to your database.',
    ...FULL,
  },
  {
    id: 'sqlserver', label: 'SQL Server', hint: 'Or Azure SQL', icon: '🗃', shape: 'sql',
    connectHint: 'A read-only login is enough — cronos never writes to your database. '
      + 'Port 1433 unless somebody changed it.',
    ...FULL,
  },
  {
    id: 'sqlite', label: 'SQLite', hint: 'A file on disk', icon: '🪶', shape: 'dsn',
    connectHint: 'A path, or file:… — cronos opens it read-only. '
      + 'Fine for a small deployment and for the demo; a file cannot be read by '
      + 'two servers at once.',
    ...FULL,
  },
  {
    id: 'clickhouse', label: 'ClickHouse', hint: 'Large event tables', icon: '⚡', shape: 'sql',
    connectHint: 'Point at the HTTP interface. Reads are streamed, not buffered.',
    ...FULL,
  },
  {
    id: 'bigquery', label: 'BigQuery', hint: 'Warehouse', icon: '◈', shape: 'sql',
    connectHint: 'Uses a service account. Set a scan limit — BigQuery bills per byte read.',
    pushdown: 'full',
    pushdownLabel: 'Filters run in the warehouse',
    pushdownHint: 'Set a cost cap on this source: an auto-refreshing dashboard on a large table can be expensive.',
  },
  {
    id: 'objectstore', label: 'Files on S3', hint: 'Parquet or CSV', icon: '🗄', shape: 'object',
    connectHint: 'Every file under the prefix is read as one table. Parquet is much faster than CSV.',
    pushdown: 'partial',
    pushdownLabel: 'Some filters run at the source',
    pushdownHint: 'Parquet skips whole files and columns that cannot match. CSV has to be read in full.',
  },
  {
    id: 'api', label: 'REST API', hint: 'JSON endpoint', icon: '⇄', shape: 'api',
    connectHint: 'cronos calls the endpoint and turns its JSON into a table you can query with SQL.',
    pushdown: 'declared',
    pushdownLabel: 'Only declared filters reach the API',
    pushdownHint: 'An API cannot take a WHERE clause. Map filters to query parameters where you can; the rest are applied after fetching, under a row cap. Results are cached — per project and per customer.',
  },
  {
    id: 'excel', label: 'Excel', hint: 'Spreadsheet', icon: '▦', shape: 'file',
    connectHint: 'The first sheet is used unless you choose another. Header row is detected.',
    pushdown: 'none',
    pushdownLabel: 'Filters run after loading',
    pushdownHint: 'The whole sheet is read, then filtered. Fine for thousands of rows, not millions.',
  },
]

/**
 * What a source kind is called in a definition's `driver` field.
 *
 * Mostly the same word, and deliberately not always: the wire name for SQL
 * Server is `sqlserver`, which is what the driver registers as, while the label
 * people read is "SQL Server". `mssql` is accepted by the engine too, because
 * it is what half the world types.
 */
export function driverFor(kind: SourceKind): string {
  return kind
}

/** Which grains a kind can bucket a date by. */
export function grainsFor(kind: SourceKind): string[] {
  // SQL Server counts week boundaries from Sunday whatever the session says,
  // so the same report bucketed weekly would disagree with every other source.
  // The engine refuses it; saying so here means somebody finds out while
  // building the report rather than when it runs.
  if (kind === 'sqlserver') return ['day', 'month', 'quarter', 'year']
  return ['day', 'week', 'month', 'quarter', 'year']
}
