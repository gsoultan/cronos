import { describe, expect, test } from 'bun:test'
import { catalogEntries, matchEntries, PAGES } from './jump'
import type { CatalogView } from './api'

// Only the fields an entry is made from.
const catalog = {
  sources: [{ name: 'warehouse', title: 'Demo warehouse', driver: 'sqlite', datasets: 2 }],
  datasets: [
    { name: 'invoices', title: 'Invoices', sources: ['warehouse'], fields: 7, measures: 1, params: 0 },
    { name: 'consumption', title: 'Consumption', sources: ['warehouse'], fields: 3, measures: 0, params: 0 },
  ],
  reports: [
    { name: 'billing-summary', title: 'Billing summary', datasets: ['invoices'], outputs: [], blocks: 5 },
    { name: 'customers', datasets: [], outputs: [], blocks: 1 },
  ],
  schedules: [{
    name: 'monthly-statements', title: 'Monthly statements', report: 'statement',
    output: 'pdf', cron: '0 6 1 * *', timezone: 'UTC', bursts: true, channels: [],
  }],
} as unknown as CatalogView

const all = [...PAGES, ...catalogEntries(catalog)]
const labels = (typed: string) => matchEntries(all, typed).map((e) => e.label)

describe('jumping to something', () => {
  test('nothing typed offers the pages', () => {
    expect(matchEntries(all, '  ').every((e) => e.kind === 'Page')).toBe(true)
  })

  test('a word inside a label beats letters inside a word', () => {
    expect(labels('sum')).toEqual(['Billing summary', 'Consumption'])
  })

  test('a label that starts with the term comes first', () => {
    expect(labels('s')[0]).toBe('Schedules')
  })

  test('the identifier is searched too', () => {
    expect(labels('monthly-st')).toEqual(['Monthly statements'])
  })

  test('a report without a title is found by its name', () => {
    expect(labels('custom')).toEqual(['customers'])
  })

  test('every entry goes to a route the portal serves, with its name as a parameter', () => {
    const report = catalogEntries(catalog).find((e) => e.kind === 'Report')
    expect(report).toMatchObject({ to: '/reports/$name', params: { name: 'billing-summary' } })
    const source = catalogEntries(catalog).find((e) => e.kind === 'Source')
    expect(source).toMatchObject({ to: '/data/sources/$name/edit', params: { name: 'warehouse' } })
  })

  test('nothing matching is an empty list, not the pages', () => {
    expect(matchEntries(all, 'zzzz')).toEqual([])
  })
})
