import { afterEach, beforeEach, expect, test } from 'bun:test'
import { QueryClient } from '@tanstack/react-query'
import { forgetOnSessionChange } from './queryClient'
import { signOut, SIGNED_IN } from './api'

/*
 * The cache belongs to a session.
 *
 * Found by driving it: two projects in one deployment, sign in as an admin of
 * one, sign out, sign in as an admin of the other in the same page load — and
 * the second one's Reports page listed the first one's reports, under the
 * second one's organisation in the header. Convincing enough that nobody would
 * report it as a bug.
 *
 * The cause is that no query key names who asked. `['catalog']` is the same key
 * for everybody, and TanStack Query hands the cached answer straight back
 * before any request goes out — with `staleTime` on it, without sending one at
 * all. The keys are not the fix, because the fix has to hold for hooks that do
 * not exist yet; see queryClient.ts.
 *
 * These use the real keys rather than invented ones, and go through the real
 * signOut() rather than dispatching the event by hand, so the test breaks if
 * either the keys or the announcement moves.
 */

let stop: () => void
let client: QueryClient
const real = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')

beforeEach(() => {
  const store = new Map<string, string>()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true, writable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
    },
  })
  client = new QueryClient()
  stop = forgetOnSessionChange(client)
})

afterEach(() => {
  stop()
  client.clear()
  if (real) Object.defineProperty(globalThis, 'localStorage', real)
  else Reflect.deleteProperty(globalThis, 'localStorage')
})

/** What one project's session leaves behind, under the keys the hooks use. */
function acmesAnswers() {
  client.setQueryData(['catalog'], { reports: [{ name: 'billing-summary' }] })
  client.setQueryData(['people'], [{ email: 'ada@acme.example' }])
  client.setQueryData(['runs', 50], [{ id: 'run_1' }])
  client.setQueryData(['definition', 'Report', 'billing-summary'], { name: 'billing-summary' })
  client.setQueryData(['report', 'billing-summary', {}], { blocks: [{ value: 41250 }] })
}

const held = () => client.getQueryCache().getAll().length

test('signing out takes the answers with it', () => {
  acmesAnswers()
  expect(held()).toBe(5)

  signOut()

  expect(held()).toBe(0)
  expect(client.getQueryData(['catalog'])).toBeUndefined()
})

/*
 * And on the way in, because a session can be replaced without one ending: an
 * SSO callback adopts a token from the URL fragment, which is the same two
 * sessions in one page load with no sign-out between them.
 */
test('and so does signing in', () => {
  acmesAnswers()

  globalThis.dispatchEvent(new Event(SIGNED_IN))

  expect(held()).toBe(0)
})

/*
 * The specific thing that was seen on screen. Not "the cache is empty" — that
 * a second session asking the question everyone asks does not get the first
 * session's answer without a request being made.
 */
test('the next session is not served the last one’s catalogue', () => {
  client.setQueryData(['catalog'], { reports: [{ name: 'billing-summary' }] })

  signOut()
  // Somebody else signs in and the Reports page mounts, asking for ['catalog'].
  const served = client.getQueryData(['catalog'])

  expect(served).toBeUndefined()
})

/*
 * A cache that is cleared on every render is no cache, and the sign-in page
 * fires nothing — so a session that is simply carrying on keeps its answers.
 */
test('a session that has not changed keeps its cache', () => {
  acmesAnswers()

  globalThis.dispatchEvent(new Event('cronos:something-else'))

  expect(held()).toBe(5)
})
