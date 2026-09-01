import type { Dataset, Report } from './types'

/** Stand-in data until the engine exists. Deterministic, so screenshots are stable. */

export const datasets: Dataset[] = [
  {
    name: 'invoices',
    label: 'Invoices',
    description: 'Issued invoices with customer detail.',
    fields: [
      { name: 'customer_name', label: 'Customer', type: 'string', role: 'dimension' },
      { name: 'issued_at', label: 'Issued date', type: 'date', role: 'dimension' },
      { name: 'due_at', label: 'Due date', type: 'date', role: 'dimension' },
      { name: 'status', label: 'Status', type: 'enum', role: 'dimension', values: ['Draft', 'Sent', 'Paid', 'Overdue'] },
      { name: 'region', label: 'Region', type: 'enum', role: 'dimension', values: ['North', 'South', 'East', 'West'] },
      { name: 'total', label: 'Amount', type: 'decimal', role: 'measure', format: 'currency' },
      { name: 'days_late', label: 'Days late', type: 'number', role: 'measure' },
    ],
  },
  {
    name: 'shipments',
    label: 'Shipments',
    description: 'Outbound shipments and delivery performance.',
    fields: [
      { name: 'carrier', label: 'Carrier', type: 'string', role: 'dimension' },
      { name: 'shipped_at', label: 'Shipped date', type: 'date', role: 'dimension' },
      { name: 'status', label: 'Status', type: 'enum', role: 'dimension', values: ['In transit', 'Delivered', 'Delayed'] },
      { name: 'weight', label: 'Weight (kg)', type: 'number', role: 'measure' },
      { name: 'cost', label: 'Cost', type: 'decimal', role: 'measure', format: 'currency' },
    ],
  },
]

export const reports: Report[] = [
  {
    name: 'monthly-invoice-statement',
    label: 'Monthly invoice statement',
    folder: 'Finance',
    description: 'Per-customer statement, mailed on the 1st.',
    dataset: 'invoices',
    tiles: [],
    filter: { id: 'root', kind: 'group', join: 'and', children: [] },
    updatedAt: '2026-08-04T09:12:00Z',
    updatedBy: 'Dewi',
    outputs: ['interactive', 'pdf', 'xlsx'],
    scheduled: { cron: '0 6 1 * *', recipients: 812 },
  },
  {
    name: 'overdue-receivables',
    label: 'Overdue receivables',
    folder: 'Finance',
    description: 'Everything past due, by region.',
    dataset: 'invoices',
    tiles: [],
    filter: { id: 'root', kind: 'group', join: 'and', children: [] },
    updatedAt: '2026-08-09T16:40:00Z',
    updatedBy: 'Marek',
    outputs: ['interactive', 'xlsx'],
  },
  {
    name: 'carrier-performance',
    label: 'Carrier performance',
    folder: 'Operations',
    description: 'Delivery times and cost per carrier.',
    dataset: 'shipments',
    tiles: [],
    filter: { id: 'root', kind: 'group', join: 'and', children: [] },
    updatedAt: '2026-07-28T11:05:00Z',
    updatedBy: 'Priya',
    outputs: ['interactive'],
    scheduled: { cron: '0 7 * * 1', recipients: 14 },
  },
  {
    name: 'quarterly-revenue',
    label: 'Quarterly revenue',
    folder: 'Executive',
    description: 'Revenue by region and quarter.',
    dataset: 'invoices',
    tiles: [],
    filter: { id: 'root', kind: 'group', join: 'and', children: [] },
    updatedAt: '2026-08-01T08:00:00Z',
    updatedBy: 'Dewi',
    outputs: ['interactive', 'pdf'],
  },
]

/* -- Result data --------------------------------------------------------- */

export const STATUSES = ['Paid', 'Sent', 'Overdue'] as const

/* Keyed `label` rather than `month`, like every other chart datum. The key
   used to be the reason the axis formatted itself as a date — see ColumnChart. */
export const billedByMonth = [
  { label: '2026-02-01', Paid: 284_000, Sent: 61_000, Overdue: 18_400 },
  { label: '2026-03-01', Paid: 312_500, Sent: 74_200, Overdue: 22_100 },
  { label: '2026-04-01', Paid: 298_100, Sent: 68_900, Overdue: 31_800 },
  { label: '2026-05-01', Paid: 341_700, Sent: 82_400, Overdue: 27_600 },
  { label: '2026-06-01', Paid: 366_200, Sent: 79_100, Overdue: 34_900 },
  { label: '2026-07-01', Paid: 388_400, Sent: 91_300, Overdue: 41_200 },
]

export const outstandingTrend = [4.9, 5.2, 4.7, 5.4, 5.1, 5.6]
export const overdueCountTrend = [512, 486, 531, 498, 477, 463]

export const collectionsTrend = [
  { month: '2026-02-01', value: 92.1 },
  { month: '2026-03-01', value: 93.4 },
  { month: '2026-04-01', value: 90.8 },
  { month: '2026-05-01', value: 94.2 },
  { month: '2026-06-01', value: 93.1 },
  { month: '2026-07-01', value: 95.6 },
]

const CUSTOMERS = [
  'Alpine Freight', 'Bergen Logistics', 'Cobalt Manufacturing', 'Delta Foods',
  'Eastward Trading', 'Fjord Systems', 'Granite Supply', 'Harbour Retail',
  'Ivory Chemical', 'Junction Rail', 'Keystone Paper', 'Lumen Energy',
]

export interface InvoiceRow {
  id: string
  customer_name: string
  issued_at: string
  due_at: string
  status: string
  region: string
  total: number
  days_late: number
}

/* Deterministic pseudo-random, so screenshots are stable but the data does not
   fall into visible cycles — a status column that repeats every fourth row
   makes a demo look like a demo. */
function rand(seed: number): number {
  const x = Math.sin(seed * 12.9898) * 43758.5453
  return x - Math.floor(x)
}

/** 4,000 rows — enough that virtualisation is doing real work. */
export const invoiceRows: InvoiceRow[] = Array.from({ length: 4000 }, (_, i) => {
  const r = rand(i + 1)
  const status = r < 0.62 ? 'Paid' : r < 0.82 ? 'Sent' : r < 0.94 ? 'Overdue' : 'Draft'
  const issued = new Date(2026, 1, 1)
  issued.setDate(issued.getDate() + Math.floor(rand(i + 500) * 180))
  const due = new Date(issued)
  due.setDate(due.getDate() + 30)
  return {
    id: `INV-${10_000 + i}`,
    customer_name: CUSTOMERS[Math.floor(rand(i + 900) * CUSTOMERS.length)]!,
    issued_at: issued.toISOString(),
    due_at: due.toISOString(),
    status,
    region: ['North', 'South', 'East', 'West'][Math.floor(rand(i + 1300) * 4)]!,
    total: Math.round(400 + rand(i + 1700) * 24_000),
    days_late: status === 'Overdue' ? 1 + Math.floor(rand(i + 2100) * 90) : 0,
  }
})
