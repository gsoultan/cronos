import { describe, expect, test } from 'bun:test'
import { retaken, textOf, toFilters } from './ReportFilters'
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

/* `control` is resolved by the server, so these carry the default each type
   gets rather than leaving it out — a payload without one is not a payload the
   server sends. */
const period: ReportFilter = { name: 'period', label: 'Period', type: 'date', control: 'range' }
const region: ReportFilter = { name: 'region', label: 'Region', type: 'string', control: 'search' }
const status: ReportFilter = {
  name: 'status', label: 'Status', type: 'enum', values: ['sent', 'overdue'],
  control: 'dropdown',
}
const amount: ReportFilter = { name: 'amount', label: 'Amount', type: 'number', control: 'range' }
const size: ReportFilter = { name: 'size', label: 'Size', type: 'number', control: 'slider' }
const depots: ReportFilter = {
  name: 'depots', label: 'Depots', type: 'enum', values: ['North', 'South', 'East'],
  control: 'checkboxes',
}
const where: ReportFilter = { name: 'where', label: 'Where', type: 'area', control: 'map' }
const paid: ReportFilter = { name: 'paid', label: 'Paid', type: 'bool', control: 'dropdown' }

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
    const out = toFilters([size], { size: ['1200'] })
    expect(out).toEqual({ size: { op: 'eq', values: [1200] } })
    expect(typeof out.size!.values[0]).toBe('number')
  })

  /*
   * A number's default control is a range, and it sent `eq` with the lower
   * end: From 100 To 500 asked for exactly 100. The same two ends a date has,
   * as numbers.
   */
  test('a number range is a range', () => {
    expect(toFilters([amount], { amount: ['100', '500'] })).toEqual({
      amount: { op: 'between', values: [100, 500] },
    })
    expect(toFilters([amount], { amount: ['', '500'] })).toEqual({
      amount: { op: 'lte', values: [500] },
    })
  })

  // Several boxes ticked sent the first of them and dropped the rest.
  test('several ticked are several', () => {
    expect(toFilters([depots], { depots: ['North', 'East'] })).toEqual({
      depots: { op: 'in', values: ['North', 'East'] },
    })
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

/*
 * A map sets filters too — a region clicked, the view it was moved to — and
 * the bar has to show them, and not undo them. The draft is text; what a map
 * sets is not always something text says.
 */
describe('a filter a map set', () => {
  const box = { op: 'within', values: [52.3, 4.8, 52.4, 5.0] }

  test('an area goes on as it was when something else is applied', () => {
    const applied = { where: box }
    const draft = { ...textOf(applied), region: ['acme'] }
    expect(toFilters([where, region], draft, applied)).toEqual({
      where: box, region: { op: 'contains', values: ['acme'] },
    })
  })

  test('and is never made from text', () => {
    expect(toFilters([where], { where: ['52.3', '4.8', '52.4', '5'] })).toEqual({})
  })

  /* A region picked is `in`. Rewritten as the bar writes a string, it would
     become `contains` — and "North" would match "North East". */
  test('a region picked keeps its operator', () => {
    const applied = { region: { op: 'in', values: ['North'] } }
    expect(toFilters([region], textOf(applied), applied)).toEqual(applied)
  })

  test('changed by the reader, it is the bar\'s again', () => {
    const applied = { region: { op: 'in', values: ['North'] } }
    expect(toFilters([region], { region: ['acme'] }, applied)).toEqual({
      region: { op: 'contains', values: ['acme'] },
    })
  })
})

describe('the draft follows what is applied', () => {
  /* The upper end alone came back in the lower box once the bar started
     following the applied filters: "until 31 July" read as "from 31 July". */
  test('the upper end of a range comes back where it was typed', () => {
    expect(textOf({ period: { op: 'lte', values: ['2026-07-31'] } })).toEqual({
      period: ['', '2026-07-31'],
    })
  })

  test('a filter the map set replaces what the draft held for it', () => {
    const before = {}
    const now = { depots: ['North'] }
    expect(retaken({ depots: ['South'] }, before, now)).toEqual({ depots: ['North'] })
  })

  test('an edit not yet applied, to another filter, stays', () => {
    const before = {}
    const now = { depots: ['North'] }
    expect(retaken({ region: ['acm'] }, before, now)).toEqual({
      region: ['acm'], depots: ['North'],
    })
  })

  test('a filter let go of leaves the draft', () => {
    const before = { where: ['52.3', '4.8', '52.4', '5'] }
    expect(retaken({ ...before }, before, {})).toEqual({})
  })
})
