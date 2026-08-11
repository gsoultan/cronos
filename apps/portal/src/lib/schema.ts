/** What the connected source exposes. Introspected from the database in the
 *  real thing; stood in here until the engine exists. */

export interface Column {
  name: string
  type: 'string' | 'number' | 'decimal' | 'date' | 'bool'
  /** Hints the builder uses to guess joins and default labels. */
  pk?: boolean
  fk?: { table: string; column: string }
}

export interface Table {
  name: string
  label: string
  rows: number
  columns: Column[]
}

export const TABLES: Table[] = [
  {
    name: 'invoices', label: 'Invoices', rows: 482_119,
    columns: [
      { name: 'id', type: 'string', pk: true },
      { name: 'customer_id', type: 'string', fk: { table: 'customers', column: 'id' } },
      { name: 'issued_at', type: 'date' },
      { name: 'due_at', type: 'date' },
      { name: 'currency', type: 'string' },
      { name: 'total_cents', type: 'number' },
      { name: 'status', type: 'string' },
    ],
  },
  {
    name: 'customers', label: 'Customers', rows: 812,
    columns: [
      { name: 'id', type: 'string', pk: true },
      { name: 'name', type: 'string' },
      { name: 'billing_email', type: 'string' },
      { name: 'region', type: 'string' },
      { name: 'status', type: 'string' },
    ],
  },
  {
    name: 'shipments', label: 'Shipments', rows: 1_204_338,
    columns: [
      { name: 'id', type: 'string', pk: true },
      { name: 'invoice_id', type: 'string', fk: { table: 'invoices', column: 'id' } },
      { name: 'carrier', type: 'string' },
      { name: 'shipped_at', type: 'date' },
      { name: 'weight_kg', type: 'decimal' },
      { name: 'cost_cents', type: 'number' },
      { name: 'status', type: 'string' },
    ],
  },
]

export const tableByName = (n: string) => TABLES.find((t) => t.name === n)

/** First letter of each word, so `invoices` → `i`, `order_lines` → `ol`. */
export function aliasFor(table: string, taken: string[]): string {
  const base = table.split('_').map((w) => w[0]).join('').toLowerCase()
  if (!taken.includes(base)) return base
  for (let i = 2; ; i++) if (!taken.includes(base + i)) return base + i
}

/**
 * A join the schema already implies. Offering the foreign key rather than
 * making someone spell `c.id = i.customer_id` is the difference between a join
 * being a click and a join being the reason they give up and ask an engineer.
 */
export function suggestedJoins(from: string): { table: string; left: string; right: string }[] {
  const out: { table: string; left: string; right: string }[] = []
  const source = tableByName(from)
  for (const c of source?.columns ?? []) {
    if (c.fk) out.push({ table: c.fk.table, left: c.name, right: c.fk.column })
  }
  for (const t of TABLES) {
    if (t.name === from) continue
    for (const c of t.columns) {
      if (c.fk?.table === from) out.push({ table: t.name, left: c.fk.column, right: c.name })
    }
  }
  return out
}
