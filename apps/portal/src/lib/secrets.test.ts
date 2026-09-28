import { describe, expect, test } from 'bun:test'
import {
  afterRemoval, afterSetting, brokenWithout, entryFor, envName, MAX_VALUE_BYTES, reference,
  referenced, referencedNames, removalConsequence, secretNameProblem, secretValueProblem,
  sourceSecret,
} from './secrets'
import type { SecretEntry } from './types'

/*
 * The name rule is the server's, restated so the Add form's button means
 * something. Every case here is one internal/platform/secret decides the same
 * way; a drift is a name the form accepts and the server refuses, which costs
 * a round trip and a sentence rather than a secret stored wrongly.
 */
describe('a secret name', () => {
  test('takes letters, digits, dots, dashes and underscores', () => {
    for (const ok of ['mapbox-token', 'warehouse_password', 'tiles-acme.v2', 'A1', '9lives']) {
      expect(secretNameProblem(ok)).toBeUndefined()
    }
  })

  /* `.` and `..` are names a reference accepts between its braces, and a
     secret read from a directory of files would read them as directories. */
  test('starts with a letter or a digit', () => {
    for (const bad of ['.', '..', '-token', '_x', '.hidden']) {
      expect(secretNameProblem(bad)).toBeDefined()
    }
  })

  test('refuses what a reference cannot carry', () => {
    for (const bad of ['has space', 'a/b', 'semi;colon', 'brace}', 'dollar$', 'é']) {
      expect(secretNameProblem(bad)).toBeDefined()
    }
  })

  test('is at most 128 characters, and is needed at all', () => {
    expect(secretNameProblem('a'.repeat(128))).toBeUndefined()
    expect(secretNameProblem('a'.repeat(129))).toBeDefined()
    expect(secretNameProblem('')).toBeDefined()
  })
})

describe('a secret value', () => {
  test('is needed, and a line ending alone is not one', () => {
    expect(secretValueProblem('')).toBeDefined()
    // The server trims what a paste from a terminal adds, then refuses nothing.
    expect(secretValueProblem('\r\n')).toBeDefined()
    expect(secretValueProblem('pk.abc\n')).toBeUndefined()
  })

  /* Measured in bytes, as the server measures it. Characters would let a value
     of multi-byte text through here and have it refused there. */
  test('is at most 16 KiB of bytes, not of characters', () => {
    expect(secretValueProblem('x'.repeat(MAX_VALUE_BYTES))).toBeUndefined()
    expect(secretValueProblem('x'.repeat(MAX_VALUE_BYTES + 1))).toBeDefined()
    expect(secretValueProblem('é'.repeat(MAX_VALUE_BYTES / 2))).toBeUndefined()
    expect(secretValueProblem('é'.repeat(MAX_VALUE_BYTES / 2 + 1))).toBeDefined()
  })

  test('keeps what is inside it, spaces included', () => {
    expect(secretValueProblem('  a password with spaces  ')).toBeUndefined()
  })
})

/* What the wizard tells somebody to set when the server cannot store the
   password itself. It has to be the variable secret.Env reads, or the advice
   is a variable nothing looks at. */
test('the environment variable is upper-cased, with dots and dashes as underscores', () => {
  expect(envName('warehouse-password')).toBe('CRONOS_SECRET_WAREHOUSE_PASSWORD')
  expect(envName('mapbox-token')).toBe('CRONOS_SECRET_MAPBOX_TOKEN')
  expect(envName('scratch-warehouse_password')).toBe('CRONOS_SECRET_SCRATCH_WAREHOUSE_PASSWORD')
  expect(envName('tiles-acme.v2')).toBe('CRONOS_SECRET_TILES_ACME_V2')
})

/* The name every connection string the portal has written already reads, so
   storing it is what makes those sources open. Changing the separator would
   orphan every one of them. */
test('a source names its own secrets after itself', () => {
  expect(sourceSecret('warehouse', 'password')).toBe('warehouse_password')
  expect(sourceSecret('lake', 'credentials')).toBe('lake_credentials')
  expect(secretNameProblem(sourceSecret('scratch-warehouse', 'password'))).toBeUndefined()
})

describe('references', () => {
  test('are written and read back', () => {
    expect(reference('lake_credentials')).toBe('${secret:lake_credentials}')
    expect(referenced('${secret:lake_credentials}')).toBe('lake_credentials')
    expect(referenced('  ${secret:x}  ')).toBe('x')
  })

  /* A key with a reference in the middle of a literal is half a secret. */
  test('are whole or they are not one', () => {
    expect(referenced('pk.${secret:x}')).toBeUndefined()
    expect(referenced('key_id=AKIA;secret=${secret:x}')).toBeUndefined()
    expect(referenced('chain')).toBeUndefined()
    expect(referenced(undefined)).toBeUndefined()
  })

  test('are all found in a tile url, once each', () => {
    const url = 'https://t.example/{z}/{x}/{y}.png?key=${secret:tiles-acme}&s=${secret:tiles-sig}&k=${secret:tiles-acme}'
    expect(referencedNames(url)).toEqual(['tiles-acme', 'tiles-sig'])
    expect(referencedNames('https://t.example/{z}/{x}/{y}.png')).toEqual([])
    expect(referencedNames(undefined)).toEqual([])
  })
})

const entry = (e: Partial<SecretEntry>): SecretEntry =>
  ({ name: 'mapbox-token', source: 'missing', usedBy: [], ...e })

test('the list is asked by name', () => {
  const list = { store: true, secrets: [entry({ name: 'a' }), entry({ name: 'b', source: 'project' })] }
  expect(entryFor(list, 'b')?.source).toBe('project')
  expect(entryFor(list, 'c')).toBeUndefined()
  expect(entryFor(undefined, 'a')).toBeUndefined()
})

/* "Broken" is two different afternoons: a warehouse that will not connect and
   a map with no basemap. The sentence says which. */
test('what breaks without a secret is said per kind', () => {
  expect(brokenWithout([{ kind: 'Report', name: 'parcel-network' }]))
    .toBe('parcel-network draws its maps without a basemap')
  expect(brokenWithout([{ kind: 'DataSource', name: 'warehouse' }]))
    .toBe('warehouse cannot connect, so the reports reading it fail')
  expect(brokenWithout([
    { kind: 'DataSource', name: 'a' }, { kind: 'DataSource', name: 'b' },
    { kind: 'Report', name: 'r1' }, { kind: 'Report', name: 'r2' },
  ])).toBe('a and b cannot connect, so the reports reading them fail; r1 and r2 draw their maps without a basemap')
})

/*
 * The confirmation says what happens, and it cannot know which of two things
 * will: the deployment may or may not have its own value behind the project's.
 * So it says both, rather than promising a fallback that may not be there.
 */
test('removing names what uses it and both ways it can go', () => {
  const said = removalConsequence(entry({
    source: 'project', usedBy: [{ kind: 'Report', name: 'parcel-network' }],
  }))
  expect(said).toContain('parcel-network will fall back to the server’s own mapbox-token')
  expect(said).toContain('draws its maps without a basemap')
  expect(removalConsequence(entry({ source: 'project' }))).toBe('Nothing uses it.')
})

/* The question after setting a warehouse password is whether somebody now has
   to ask for a deploy. */
test('storing one says who reads it, and that nothing restarts', () => {
  expect(afterSetting('mapbox-token', entry({ usedBy: [{ kind: 'Report', name: 'parcel-network' }] })))
    .toBe('Stored mapbox-token. parcel-network reads it from now on; nothing needs restarting.')
  expect(afterSetting('spare', undefined))
    .toBe('Stored spare. Nothing reads it yet — a definition names it as ${secret:spare}.')
})

/* After the fact, from the list the server now holds rather than a guess. */
test('what a removal did is read from where the name stands now', () => {
  const uses = [{ kind: 'Report' as const, name: 'parcel-network' }]
  expect(afterRemoval('mapbox-token', entry({ source: 'deployment', usedBy: uses })))
    .toBe('Removed mapbox-token. parcel-network now reads the server’s own value.')
  expect(afterRemoval('mapbox-token', entry({ source: 'missing', usedBy: uses })))
    .toContain('Nothing answers it now')
  // Stored, used by nothing, and now gone from the list altogether.
  expect(afterRemoval('old-token', undefined)).toBe('Removed old-token.')
})
