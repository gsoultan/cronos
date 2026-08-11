import { expect, test } from 'bun:test'
import { monthLabel, shortDate } from './format'

test('a date becomes a month and a year', () => {
  expect(monthLabel('2026-07-01')).toBe('Jul ’26')
})

/* A chart's x axis is whatever it was grouped by, and grouping by customer is
   as ordinary as grouping by month. Formatting a customer's name as a date
   throws from Intl, which takes the page down rather than one label with it —
   the crash that "Invalid time value" on a live report turned out to be. */
test('anything that is not a date is left as it is', () => {
  for (const given of ['Aurora Freight', 'Rotterdam', '', 'Q3', 'not a date']) {
    expect(monthLabel(given)).toBe(given)
    expect(shortDate(given)).toBe(given)
  }
})
