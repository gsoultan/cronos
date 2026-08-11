import { describe, expect, test } from 'bun:test'
import { paginate } from '../components/Pagination'

const items = Array.from({ length: 14 }, (_, i) => i)

describe('paginate', () => {
  test('slices the requested page', () => {
    expect(paginate(items, 0, 6).slice).toEqual([0, 1, 2, 3, 4, 5])
    expect(paginate(items, 1, 6).slice).toEqual([6, 7, 8, 9, 10, 11])
  })

  test('the last page may be short', () => {
    expect(paginate(items, 2, 6).slice).toEqual([12, 13])
  })

  test('clamps a page that ran off the end rather than returning nothing', () => {
    // Filtering a list shorter than the current page is the common way to land
    // on an empty page and conclude there are no results.
    const { slice, page } = paginate(items, 9, 6)
    expect(page).toBe(2)
    expect(slice).toEqual([12, 13])
  })

  test('an empty list is page zero, not page minus one', () => {
    const { slice, page } = paginate([], 3, 6)
    expect(page).toBe(0)
    expect(slice).toEqual([])
  })

  test('a list that fits is entirely on page zero', () => {
    expect(paginate([1, 2, 3], 0, 6).slice).toEqual([1, 2, 3])
  })
})
