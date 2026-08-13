import { describe, expect, test } from 'bun:test'
import { SOURCE_KINDS, type SourceKind } from './sources'
import { readDataSource } from './definitions'

/*
 * Every driver the engine accepts has to reach a card, or the form dies.
 *
 * DataSourceForm looked its kind up with `.find(...)!` — an assertion that a
 * lookup always succeeds. A source stored with `driver: sqlite`, which the
 * engine supports and the demo ships, matched nothing: three ticked steps, a
 * highlighted Connect, and no fields at all.
 *
 * The form now falls back to a generic SQL card, so a stored driver can never
 * produce an empty screen again. These are the tests that the mapping is right
 * for the drivers we do know, since the fallback is a safety net rather than
 * somewhere to land routinely.
 */

// internal/core/definition/datasourcevalidate.go, which is the list the engine
// will actually open a source with.
const ACCEPTED = ['postgres', 'mysql', 'sqlite', 'duckdb', 'object-store', 'sqlserver', 'mssql']

function kindOf(driver: string): string {
  const doc = [
    'apiVersion: cronos.dev/v1',
    'kind: DataSource',
    'metadata:',
    '  name: warehouse',
    'spec:',
    `  driver: ${driver}`,
    '  dsn: "file:demo.db"',
  ].join('\n')
  return readDataSource(doc).input.kind
}

describe('a stored driver reaches a card', () => {
  for (const driver of ACCEPTED) {
    test(driver, () => {
      const kind = kindOf(driver)
      expect(kind).not.toBe('')

      // duckdb is behind a build tag and is deliberately not offered — a card
      // for a driver this binary may not have would be a lie. It still has to
      // map to *something* so the form's fallback can name it.
      if (driver === 'duckdb') return

      const spec = SOURCE_KINDS.find((k) => k.id === (kind as SourceKind))
      expect(spec, `${driver} maps to kind ${kind}, which no card matches`).toBeDefined()
    })
  }
})

// Both spellings of SQL Server land on one card, because the engine accepts
// both and somebody who typed the other should not get a different screen.
test('mssql and sqlserver are the same card', () => {
  expect(kindOf('mssql')).toBe('sqlserver')
  expect(kindOf('sqlserver')).toBe('sqlserver')
})

// A driver nobody has heard of still produces a kind rather than an empty
// string, so the form has something to name in its fallback.
test('an unknown driver still names itself', () => {
  expect(kindOf('cassandra')).toBe('cassandra')
})
