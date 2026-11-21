import { describe, expect, test } from 'bun:test'
import { explainTargets, suggestions } from './explain'
import type { ReportBlock } from './api'

const blocks = [
  { kind: 'stat', title: 'Total billed', value: '$1.2M' },
  { kind: 'chart', chart: 'bar', title: 'Billed by month', series: [] },
  { kind: 'chart', chart: 'map', title: '', series: [] },
  { kind: 'table', title: 'Invoices', columns: [], rows: [] },
] as unknown as ReportBlock[]

describe('explainTargets', () => {
  test('the whole report first, then every block by its place', () => {
    expect(explainTargets(blocks)).toEqual([
      { label: 'The whole report' },
      { block: 0, label: 'Total billed — figure' },
      { block: 1, label: 'Billed by month — bar chart' },
      { block: 2, label: 'map' },
      { block: 3, label: 'Invoices — table' },
    ])
  })

  test('a report with no blocks can still be asked about', () => {
    expect(explainTargets([])).toEqual([{ label: 'The whole report' }])
  })
})

test('suggestions name what is being asked about', () => {
  expect(suggestions({ label: 'The whole report' })[0]).toBe('What does this report show?')
  expect(suggestions({ block: 1, label: 'x' })[0]).toBe('What does this show?')
})
