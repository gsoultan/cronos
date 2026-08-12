import { describe, expect, test } from 'bun:test'
import { formatSecret, readiness } from './security'
import type { Person } from './people'

const person = (name: string, twoFactor: boolean, isYou = false): Person => ({
  id: name, name, email: `${name}@x.test`, orgId: 'o', orgRole: 'member',
  projectRoles: {}, twoFactor, isYou,
})


const on = (p: Person) => !!p.twoFactor

describe('readiness', () => {

  test('counts coverage and names who would lose access', () => {
    const members = [person('a', true), person('b', false), person('c', false)]
    const r = readiness(members, on)
    expect(r.covered).toBe(1)
    expect(r.total).toBe(3)
    expect(r.wouldLoseAccess.map((p) => p.name)).toEqual(['b', 'c'])
  })

  test('reports whether you would lock yourself out first', () => {
    expect(readiness([person('you', false, true)], on).youAreCovered).toBe(false)
    expect(readiness([person('you', true, true)], on).youAreCovered).toBe(true)
  })

  test('nobody is covered when there are no members', () => {
    expect(readiness([], on).youAreCovered).toBe(false)
  })
})

/*
 * The secret and the recovery codes used to be made here, in the browser, from
 * a seeded generator — and stored nowhere. Both now come from the server: one
 * from crypto/rand, the other hashed before it is written, and neither is
 * something a page can invent. What is left in this module is formatting.
 */

/*
 * Grouping the key for somebody typing it by hand.
 *
 * The only part of enrolment still done in the browser. A 32-character base32
 * string read off a screen and typed into a phone is where people lose their
 * place, and four-character groups are what every authenticator app's own
 * manual-entry screen shows.
 */
describe('formatSecret', () => {
  test('four at a time', () => {
    expect(formatSecret('ABCDEFGHIJKLMNOP')).toBe('ABCD EFGH IJKL MNOP')
  })

  test('a ragged tail is left as it is rather than padded', () => {
    expect(formatSecret('ABCDEFG')).toBe('ABCD EFG')
  })

  test('nothing in, nothing out', () => {
    expect(formatSecret('')).toBe('')
  })
})
