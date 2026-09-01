import { describe, expect, test } from 'bun:test'
import { bands } from './LiveReport'
import { monthLabel } from '../lib/format'

/*
 * A chart axis says what the server said.
 *
 * The bug this pins was silent, which is the only kind worth this much comment.
 * The datum a bar chart takes had a key called `month`, so the axis formatted
 * every label as a date. That is right for the one report the fixture holds and
 * wrong for every report grouped by anything else — and JavaScript does not
 * complain, because `new Date("c-1")` is not an error. It is the 1st of January
 * 2001.
 *
 * So a report built in the browser, grouping shipment weight by customer,
 * rendered three real totals over three invented months. Nothing threw, nothing
 * logged, and the numbers were correct — which is what makes it worse than a
 * crash: the chart was wrong in the one place a reader trusts without checking.
 *
 * Found by building that report through the portal and looking at it.
 */
describe('a chart band keeps the label the engine wrote', () => {
  test('a customer id is not a month', () => {
    const out = bands([
      { label: 'c-1', value: 2760.5, formatted: '2,760.50' },
      { label: 'c-2', value: 5550, formatted: '5,550' },
      { label: 'c-3', value: 1820, formatted: '1,820' },
    ])
    expect(out.map((b) => b.label)).toEqual(['c-1', 'c-2', 'c-3'])
  })

  test('an already-formatted period is passed through, not re-formatted', () => {
    // The engine writes "May 2026" because it is the thing that knew the grain.
    // Re-formatting would be a second opinion from the side with less
    // information, and in the worst case a different month.
    const out = bands([
      { label: 'May 2026', value: 1, formatted: '1' },
      { label: 'Jun 2026', value: 2, formatted: '2' },
    ])
    expect(out.map((b) => b.label)).toEqual(['May 2026', 'Jun 2026'])
  })

  test('values are untouched', () => {
    expect(bands([{ label: 'North', value: 0, formatted: '0' }]))
      .toEqual([{ label: 'North', value: 0 }])
  })
})

/*
 * And the reason the default has to be identity rather than "format if it
 * parses".
 *
 * These are the labels a real report produces, run through the formatter that
 * used to be unconditional. Two of the four are quietly wrong, and neither
 * looks wrong on the page.
 */
describe('why the axis cannot guess', () => {
  test('a date formatter mangles ordinary labels', () => {
    expect(monthLabel('c-1')).not.toBe('c-1')   // → Jan ’01
    expect(monthLabel('12')).not.toBe('12')     // → Dec ’01
  })

  test('and leaves others alone, which is how it went unnoticed', () => {
    // A region or a status is unparseable, so those charts were always right.
    // Only the labels that happened to look like dates were corrupted, which
    // is the worst possible distribution for noticing.
    expect(monthLabel('North')).toBe('North')
    expect(monthLabel('Paid')).toBe('Paid')
  })
})
