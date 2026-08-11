import type { SourceKind } from './sources'

export interface ConnectedSource {
  id: string
  name: string
  kind: SourceKind
  detail: string
  ok: boolean
  datasets: number
}

/**
 * Enough rows that search and paging are doing real work rather than being
 * demonstrated on three items — a list that always fits hides every bug those
 * two features have.
 */
export const connectedSources: ConnectedSource[] = [
  { id: 's1', name: 'Production warehouse', kind: 'postgres', detail: 'db.internal.acme.com · 24 tables', ok: true, datasets: 6 },
  { id: 's2', name: 'Replica warehouse', kind: 'postgres', detail: 'replica.internal.acme.com · read-only', ok: true, datasets: 2 },
  { id: 's3', name: 'Event lake', kind: 'objectstore', detail: 's3://acme-lake/events/ · parquet', ok: true, datasets: 3 },
  { id: 's4', name: 'Archive lake', kind: 'objectstore', detail: 's3://acme-archive/2019-2024/ · parquet', ok: true, datasets: 0 },
  { id: 's5', name: 'Billing API', kind: 'api', detail: 'api.billing.acme.com · cached 5 min', ok: false, datasets: 1 },
  { id: 's6', name: 'Shipping API', kind: 'api', detail: 'api.ship.acme.com · cached 15 min', ok: true, datasets: 2 },
  { id: 's7', name: 'CRM (MySQL)', kind: 'mysql', detail: 'crm-db.acme.com · 41 tables', ok: true, datasets: 4 },
  { id: 's8', name: 'Product events', kind: 'clickhouse', detail: 'ch.internal.acme.com · 2.1bn rows', ok: true, datasets: 3 },
  { id: 's9', name: 'Finance warehouse', kind: 'bigquery', detail: 'acme-fin.eu · scan cap 200 GB', ok: true, datasets: 5 },
  { id: 's10', name: 'Budget workbook', kind: 'excel', detail: 'finance/budget-2026.xlsx · sheet 1', ok: true, datasets: 1 },
  { id: 's11', name: 'Headcount workbook', kind: 'excel', detail: 'people/headcount.xlsx · sheet “Live”', ok: false, datasets: 0 },
  { id: 's12', name: 'Support tickets', kind: 'api', detail: 'api.support.acme.com · cached 10 min', ok: true, datasets: 2 },
  { id: 's13', name: 'Legacy Oracle mirror', kind: 'postgres', detail: 'mirror.acme.com · nightly copy', ok: true, datasets: 1 },
  { id: 's14', name: 'Marketing lake', kind: 'objectstore', detail: 's3://acme-marketing/ · csv', ok: true, datasets: 0 },
]

export interface ListedDataset {
  id: string
  name: string
  label: string
  description: string
  source: string
  fields: number
  measures: boolean
  updatedAt: string
}

export const listedDatasets: ListedDataset[] = [
  { id: 'd1', name: 'invoices', label: 'Invoices', description: 'Issued invoices with customer detail.', source: 'Production warehouse', fields: 7, measures: true, updatedAt: '2026-08-09T09:00:00Z' },
  { id: 'd2', name: 'shipments', label: 'Shipments', description: 'Outbound shipments and delivery performance.', source: 'Production warehouse', fields: 5, measures: true, updatedAt: '2026-08-07T14:20:00Z' },
  { id: 'd3', name: 'active-customers', label: 'Active customers', description: 'One row per customer that should receive a statement.', source: 'Production warehouse', fields: 3, measures: false, updatedAt: '2026-08-02T11:05:00Z' },
  { id: 'd4', name: 'overdue-invoices', label: 'Overdue invoices', description: 'Past due, with days late and region.', source: 'Production warehouse', fields: 6, measures: true, updatedAt: '2026-08-10T08:40:00Z' },
  { id: 'd5', name: 'carrier-costs', label: 'Carrier costs', description: 'Cost per carrier per month.', source: 'Shipping API', fields: 4, measures: true, updatedAt: '2026-07-28T16:12:00Z' },
  { id: 'd6', name: 'delivery-times', label: 'Delivery times', description: 'Days in transit by carrier and region.', source: 'Shipping API', fields: 5, measures: true, updatedAt: '2026-07-30T10:02:00Z' },
  { id: 'd7', name: 'page-views', label: 'Page views', description: 'Product events rolled up per day.', source: 'Product events', fields: 4, measures: true, updatedAt: '2026-08-05T07:30:00Z' },
  { id: 'd8', name: 'signups', label: 'Signups', description: 'New accounts by source and plan.', source: 'Product events', fields: 5, measures: true, updatedAt: '2026-08-06T13:44:00Z' },
  { id: 'd9', name: 'revenue-by-region', label: 'Revenue by region', description: 'Recognised revenue, quarterly.', source: 'Finance warehouse', fields: 4, measures: true, updatedAt: '2026-08-01T09:15:00Z' },
  { id: 'd10', name: 'budget-vs-actual', label: 'Budget vs actual', description: 'Plan against outturn, by cost centre.', source: 'Budget workbook', fields: 6, measures: true, updatedAt: '2026-07-21T15:50:00Z' },
  { id: 'd11', name: 'accounts', label: 'Accounts', description: 'CRM accounts with owner and stage.', source: 'CRM (MySQL)', fields: 8, measures: false, updatedAt: '2026-08-04T12:00:00Z' },
  { id: 'd12', name: 'open-tickets', label: 'Open tickets', description: 'Unresolved support tickets by age.', source: 'Support tickets', fields: 5, measures: true, updatedAt: '2026-08-11T06:10:00Z' },
]
