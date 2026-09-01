import { afterEach, beforeEach, expect, mock, test } from 'bun:test'
import { ApiError, confirmFactor, listRuns, removeFactor } from './api'

/*
 * A wrong second-factor code is not an ended session.
 *
 * Both are 401, and until this the portal drew the same conclusion from both:
 * clear the session and show the sign-in page. So mistyping one digit while
 * enrolling threw away the enrolment and put you back at the sign-in form with
 * no explanation — and the server's own sentence, written to be read ("That
 * code is not right. Check the app is showing the current one."), was never
 * shown to anybody.
 *
 * It is not an unlikely path. A code lives thirty seconds, and reading six
 * digits off a phone and pressing a button is how you spend some of them; the
 * browser check for enrolment failed about half the time for exactly this,
 * on a code that expired between being read and being sent.
 */

const real = {
  fetch: globalThis.fetch,
  api: process.env.VITE_CRONOS_API,
  storage: Object.getOwnPropertyDescriptor(globalThis, 'localStorage'),
}

let store: Map<string, string>

beforeEach(() => {
  store = new Map()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true, writable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
    },
  })
  process.env.VITE_CRONOS_API = 'https://cronos.example'
  store.set('cronos.token', 'session-token')
  store.set('cronos.user', JSON.stringify({ id: 'u1', email: 'ada@example.com' }))
})

afterEach(() => {
  globalThis.fetch = real.fetch
  if (real.api === undefined) delete process.env.VITE_CRONOS_API
  else process.env.VITE_CRONOS_API = real.api
  if (real.storage) Object.defineProperty(globalThis, 'localStorage', real.storage)
  else Reflect.deleteProperty(globalThis, 'localStorage')
})

function refuses(message: string) {
  globalThis.fetch = mock(async () =>
    new Response(JSON.stringify({ error: message }), { status: 401 })) as unknown as typeof fetch
}

const signedIn = () => store.get('cronos.token') !== undefined

test('a wrong code while enrolling keeps the session', async () => {
  refuses('That code is not right. Check the app is showing the current one.')

  const err = await confirmFactor('000000').catch((e: unknown) => e)

  expect(err).toBeInstanceOf(ApiError)
  // The sentence reaches the page, which is the whole point of writing it.
  expect((err as ApiError).message).toContain('not right')
  expect(signedIn()).toBe(true)
})

test('and so does a code that was already used', async () => {
  refuses('That code has already been used. Wait for the next one.')

  await removeFactor('111111').catch(() => {})

  expect(signedIn()).toBe(true)
})

/*
 * The rule it is an exception to still holds. A 401 from anything that is not
 * checking a code is a session that has ended, and leaving somebody on a page
 * of "unauthorised" with no way out is the thing signOut-on-401 exists to
 * prevent.
 */
test('a 401 from anywhere else still ends the session', async () => {
  refuses('Sign in to view this.')

  await listRuns(50).catch(() => {})

  expect(signedIn()).toBe(false)
})
