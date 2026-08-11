import { describe, expect, test } from 'bun:test'
import { formatSecret, makeRecoveryCodes, makeSecret, readiness } from './security'
import type { Person } from './people'

const person = (name: string, twoFactor: boolean, isYou = false): Person => ({
  id: name, name, email: `${name}@x.test`, orgId: 'o', orgRole: 'member',
  projectRoles: {}, twoFactor, isYou,
})

describe('makeRecoveryCodes', () => {
  test('produces ten distinct codes', () => {
    // The first version multiplied 32-bit values with `*`, overflowed 2^53, and
    // returned ten identical codes — which would have shipped as one usable
    // recovery code instead of ten.
    const codes = makeRecoveryCodes('seed')
    expect(codes).toHaveLength(10)
    expect(new Set(codes).size).toBe(10)
  })

  test('uses more than one symbol', () => {
    const chars = new Set(makeRecoveryCodes('seed').join('').replace(/-/g, ''))
    expect(chars.size).toBeGreaterThan(8)
  })

  test('is deterministic for a seed and different across seeds', () => {
    expect(makeRecoveryCodes('a')).toEqual(makeRecoveryCodes('a'))
    expect(makeRecoveryCodes('a')).not.toEqual(makeRecoveryCodes('b'))
  })
})

describe('makeSecret', () => {
  test('is 32 characters of base32 with real variety', () => {
    const s = makeSecret('x')
    expect(s).toHaveLength(32)
    expect(s).toMatch(/^[A-Z2-7]+$/)
    expect(new Set(s).size).toBeGreaterThan(8)
  })

  test('groups into readable blocks without losing characters', () => {
    const s = makeSecret('x')
    expect(formatSecret(s).replace(/ /g, '')).toBe(s)
  })
})

describe('readiness', () => {
  const on = (p: Person) => !!p.twoFactor

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
