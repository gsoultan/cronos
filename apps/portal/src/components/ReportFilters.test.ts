import { describe, expect, test } from 'bun:test'
import { toFilters } from './ReportFilters'
import type { ReportFilter } from '../lib/api'

/*
 * Turning controls into a request.
 *
 * The live report had no filter bar at all, so nothing here was ever exercised
 * against a server. What matters is which operator each type means and — more
 * easily got wrong — that a control somebody never touched sends nothing:
 * `contains ""` matches every row and `between "" ""` is an error, and both
 * would be a filter nobody set changing what they see.
 */

const period: ReportFilter = { name: 'period', label: 'Period', type: 'date' }
const region: ReportFilter = { name: 'region', label: 'Region', type: 'string' }
const status: ReportFilter = {
  name: 'status', label: 'Status', type: 'enum', values: ['sent', 'overdue'],
}
const amount: ReportFilter = { name: 'amount', label: 'Amount', type: 'number' }
const paid: ReportFilter = { name: 'paid', label: 'Paid', type: 'bool' }

describe('an untouched control sends nothing', () => {
  test('no draft at all', () => {
    expect(toFilters([period, region, status, amount, paid], {})).toEqual({})
  })

  test('empty strings, which is what a cleared input holds', () => {
    expect(toFilters([period, region], { period: ['', ''], region: [''] })).toEqual({})
  })

  test('and whitespace, which is what a fat thumb holds', () => {
    expect(toFilters([region], { region: ['   '] })).toEqual({})
  })
})

describe('a date is a range', () => {
  test('both ends', () => {
    expect(toFilters([period], { period: ['2026-07-01', '2026-07-31'] })).toEqual({
      period: { op: 'between', values: ['2026-07-01', '2026-07-31'] },
    })
  })

  /*
   * One end is still a filter, and the operator says which end. Inventing the
   * other — today, or the epoch — narrows more than was asked and does it
   * silently.
   */
  test('from only', () => {
    expect(toFilters([period], { period: ['2026-07-01', ''] })).toEqual({
      period: { op: 'gte', values: ['2026-07-01'] },
    })
  })

  test('to only', () => {
    expect(toFilters([period], { period: ['', '2026-07-31'] })).toEqual({
      period: { op: 'lte', values: ['2026-07-31'] },
    })
  })
})

describe('every other type has one obvious meaning', () => {
  test('a string contains, because that is what typing a word means', () => {
    expect(toFilters([region], { region: ['acme'] })).toEqual({
      region: { op: 'contains', values: ['acme'] },
    })
  })

  test('an enum is in, so the control can grow to several without a new shape', () => {
    expect(toFilters([status], { status: ['overdue'] })).toEqual({
      status: { op: 'in', values: ['overdue'] },
    })
  })

  test('a number is a number, not the text of one', () => {
    const out = toFilters([amount], { amount: ['1200'] })
    expect(out).toEqual({ amount: { op: 'eq', values: [1200] } })
    expect(typeof out.amount!.values[0]).toBe('number')
  })

  test('a bool is a bool, and "false" is not truthy', () => {
    expect(toFilters([paid], { paid: ['false'] })).toEqual({
      paid: { op: 'eq', values: [false] },
    })
    expect(toFilters([paid], { paid: ['true'] })).toEqual({
      paid: { op: 'eq', values: [true] },
    })
  })
})

// A draft entry for a filter the report does not declare is ignored: the loop
// is over the declared filters, so stale state from a previous report cannot
// reach the server as a filter nobody asked for.
test('only the filters the report declares are sent', () => {
  expect(toFilters([region], { region: ['acme'], gone: ['x'] })).toEqual({
    region: { op: 'contains', values: ['acme'] },
  })
})
