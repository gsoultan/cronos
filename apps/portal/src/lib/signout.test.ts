import { afterEach, beforeEach, expect, mock, test } from 'bun:test'
import { endSession, SIGNED_OUT } from './api'

/*
 * Signing out is two things in an order that matters.
 *
 * The route that says where to go next is authenticated by the very session it
 * is ending, so clearing storage first would make the call fail and leave the
 * identity provider's session open — a sign-out that looks like it worked and
 * did half of what it says.
 *
 * There is no DOM here, so the three globals this touches are stood up by hand.
 * That is the point of the module reading them off globalThis rather than
 * closing over them at import time.
 */

const real = {
  fetch: globalThis.fetch,
  api: process.env.VITE_CRONOS_API,
  storage: Object.getOwnPropertyDescriptor(globalThis, 'localStorage'),
  location: Object.getOwnPropertyDescriptor(globalThis, 'location'),
}

let store: Map<string, string>
let went: string[]

function stub<T>(name: string, value: T) {
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value })
}

beforeEach(() => {
  store = new Map()
  went = []

  stub('localStorage', {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, v),
    removeItem: (k: string) => void store.delete(k),
  })
  stub('location', { assign: (to: string) => void went.push(to) })
  delete process.env.VITE_CRONOS_API
})

afterEach(() => {
  globalThis.fetch = real.fetch
  if (real.api === undefined) delete process.env.VITE_CRONOS_API
  else process.env.VITE_CRONOS_API = real.api
  for (const [name, was] of [['localStorage', real.storage], ['location', real.location]] as const) {
    if (was) Object.defineProperty(globalThis, name, was)
    else Reflect.deleteProperty(globalThis, name)
  }
})

/** A signed-in browser: a server to talk to and a session to talk with. */
function connect(token = 'session-token') {
  // Where the server is comes from the build, not from storage — the portal is
  // a static bundle pointed at one deployment.
  process.env.VITE_CRONOS_API = 'https://cronos.example'
  store.set('cronos.token', token)
  store.set('cronos.user', JSON.stringify({ id: 'u1', email: 'ada@example.com' }))
}

function answers(body: unknown, status = 200) {
  const calls: RequestInit[] = []
  globalThis.fetch = mock(async (_url: unknown, init: RequestInit = {}) => {
    calls.push(init)
    return new Response(typeof body === 'string' ? body : JSON.stringify(body), { status })
  }) as unknown as typeof fetch
  return calls
}

test('the sign-out is asked for before the token is thrown away', async () => {
  connect()
  const calls = answers({ redirect: '' })

  await endSession()

  // Not "a call was made" — a call that carried the session. Clearing first
  // sends `Bearer null`, which the server answers 401 to, which this code
  // swallows, which is a sign-out that silently skipped the provider.
  expect(calls).toHaveLength(1)
  expect(new Headers(calls[0]?.headers).get('authorization')).toBe('Bearer session-token')
  expect(calls[0]?.method).toBe('POST')
})

test('the browser goes to the provider when there is somewhere to go', async () => {
  connect()
  answers({ redirect: 'https://idp.example/logout?client_id=portal' })

  await endSession()

  expect(went).toEqual(['https://idp.example/logout?client_id=portal'])
  expect(store.get('cronos.token')).toBeUndefined()
})

test('and stays put when the provider publishes no sign-out', async () => {
  connect()
  answers({ redirect: '' })

  await endSession()

  expect(went).toEqual([])
  expect(store.get('cronos.token')).toBeUndefined()
})

/*
 * A deployment with no SSO answers 404, which is not a failure to report.
 * There is no second session to end, and the local one is over either way.
 */
test('a server without SSO still signs somebody out', async () => {
  connect()
  answers('no such endpoint', 404)

  await endSession()

  expect(store.get('cronos.token')).toBeUndefined()
  expect(went).toEqual([])
})

/*
 * And when the network is down. Leaving somebody signed in because a fetch
 * threw is the worse of the two outcomes: the part that needs the server is
 * the provider's session, and the part that does not is this one.
 */
test('an unreachable server still signs somebody out', async () => {
  connect()
  globalThis.fetch = mock(async () => {
    throw new TypeError('Failed to fetch')
  }) as unknown as typeof fetch

  await endSession()

  expect(store.get('cronos.token')).toBeUndefined()
  expect(store.get('cronos.user')).toBeUndefined()
})

// The shell listens for this to swap in the sign-in page. Without it React has
// already decided what to render, and every panel shows "unauthorised" with no
// way out.
test('the shell is told the session ended', async () => {
  connect()
  answers({ redirect: '' })

  const heard = mock(() => {})
  globalThis.addEventListener(SIGNED_OUT, heard)
  await endSession()
  globalThis.removeEventListener(SIGNED_OUT, heard)

  expect(heard).toHaveBeenCalledTimes(1)
})

// Nobody is signed in, or this build talks to no server. Clearing what is
// there is the whole of it: a fetch with no base URL would be a request to the
// portal's own origin, which is a 404 in somebody's access log per sign-out.
test('signing out of nothing needs no server', async () => {
  const calls = answers({ redirect: '' })

  await endSession()

  expect(calls).toHaveLength(0)
})
