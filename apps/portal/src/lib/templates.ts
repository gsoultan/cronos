import type { Dataset, Tile } from './types'

export interface Template {
  id: string
  label: string
  hint: string
  icon: string
  outputs: string[]
  build: (dataset: Dataset) => Omit<Tile, 'id'>[]
}

const visible = (d: Dataset) => d.fields.filter((f) => !f.hidden)
const measure = (d: Dataset) => visible(d).find((f) => f.role === 'measure')?.name
const dimension = (d: Dataset) => visible(d).find((f) => f.role === 'dimension')?.name
const dateField = (d: Dataset) => visible(d).find((f) => f.type === 'date')?.name ?? dimension(d)

/**
 * Starting points, not types.
 *
 * There is one artifact — a report — and a dashboard is a report whose only
 * output is interactive. But "dashboard" is the word people search for, so it
 * has to exist somewhere in the product. Templates are where: they preset the
 * outputs and a plausible layout, and leave nothing behind in the model.
 *
 * See docs/report-format.md § There is no separate Dashboard kind.
 */
export const TEMPLATES: Template[] = [
  {
    id: 'dashboard',
    label: 'Dashboard',
    hint: 'Headline numbers and charts, for a screen',
    icon: '▦',
    outputs: ['interactive'],
    build: (d) => [
      { kind: 'stat', title: 'Total', span: 3, field: measure(d), aggregate: 'sum' },
      { kind: 'stat', title: 'Average', span: 3, field: measure(d), aggregate: 'avg' },
      { kind: 'stat', title: 'Count', span: 3, field: measure(d), aggregate: 'count' },
      { kind: 'stat', title: 'Highest', span: 3, field: measure(d), aggregate: 'max' },
      { kind: 'bar', title: 'By month', span: 6, field: measure(d), groupBy: dateField(d), aggregate: 'sum' },
      { kind: 'line', title: 'Trend', span: 6, field: measure(d), groupBy: dateField(d), aggregate: 'sum' },
    ],
  },
  {
    id: 'statement',
    label: 'Statement',
    hint: 'A document to send out, page by page',
    icon: '▤',
    outputs: ['interactive', 'pdf'],
    build: (d) => [
      { kind: 'stat', title: 'Total due', span: 4, field: measure(d), aggregate: 'sum' },
      { kind: 'table', title: 'Detail', span: 12, columns: visible(d).slice(0, 6).map((f) => f.name) },
    ],
  },
  {
    id: 'export',
    label: 'Data export',
    hint: 'Every row, for someone to work with',
    icon: '⤓',
    outputs: ['interactive', 'xlsx'],
    build: (d) => [
      { kind: 'table', title: 'All rows', span: 12, columns: visible(d).map((f) => f.name) },
    ],
  },
]
