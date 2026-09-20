import { afterEach, beforeEach, expect, test } from 'bun:test'
import { fromSession } from './WorkspaceContext'
import { canEdit, effectiveRole } from './workspace'

/*
 * What a session says somebody may do.
 *
 * cronos has two roles and they do not agree: an owner or an admin of the
 * organisation holds project administrator in every project in it, with no
 * membership in any of them — principal.effective() on the server side.
 *
 * The portal read the project role alone and copied it upwards, so somebody
 * who administers the whole organisation was shown the interface of a viewer:
 * no way to connect a datasource, no way to say who may open a report, while
 * every endpoint behind those controls would have answered yes. The session
 * has carried `orgRole` the whole time.
 */

const real = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')
const store = new Map<string, string>()

beforeEach(() => {
  store.clear()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    writable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
    },
  })
  // connected() reads this; without it fromSession answers for sample data.
  process.env.VITE_CRONOS_API = 'http://localhost:8787'
})

afterEach(() => {
  if (real) Object.defineProperty(globalThis, 'localStorage', real)
  delete process.env.VITE_CRONOS_API
})

function signedIn(user: Record<string, unknown>) {
  store.set('cronos.token', 'v1.token.signature')
  store.set('cronos.user', JSON.stringify({
    id: 'u1', email: 'ada@acme.example', org: 'acme', project: 'finance', ...user,
  }))
}

test('an org owner with no project role administers the project', () => {
  signedIn({ role: '', orgRole: 'owner' })

  const w = fromSession()
  expect(w).not.toBeNull()
  expect(effectiveRole(w!.org, w!.project)).toBe('admin')
  expect(canEdit(w!.org, w!.project)).toBe(true)
})

// The tier below, which reaches every project the same way.
test('an org admin does too', () => {
  signedIn({ role: '', orgRole: 'admin' })

  const w = fromSession()!
  expect(effectiveRole(w.org, w.project)).toBe('admin')
})

/*
 * And an ordinary member is decided by their project role, whatever the
 * organisation says. "member" is the org role everybody who is not an
 * administrator has, so reading it as anything would promote the whole
 * deployment.
 */
test('an org member is whatever their project role says', () => {
  for (const [role, edits] of [['viewer', false], ['editor', true], ['admin', true]] as const) {
    signedIn({ role, orgRole: 'member' })

    const w = fromSession()!
    expect(effectiveRole(w.org, w.project)).toBe(role)
    expect(canEdit(w.org, w.project)).toBe(edits)
  }
})

/*
 * A session with no project role and no org role is a viewer, not an
 * administrator. The mapping refuses anything it does not recognise rather
 * than passing it through — an unknown role arriving from a newer server must
 * not be read as a permission this build does not understand.
 */
test('a role this build does not know grants nothing', () => {
  signedIn({ role: 'auditor', orgRole: 'superuser' })

  const w = fromSession()!
  expect(w.org.role).toBe('member')
  expect(effectiveRole(w.org, w.project)).toBeNull()
  expect(canEdit(w.org, w.project)).toBe(false)
})

// No session at all is the sample directory's problem, not this one's.
test('nobody signed in is nobody', () => {
  expect(fromSession()).toBeNull()
})
