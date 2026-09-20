import { afterEach, beforeEach, expect, mock, test } from 'bun:test'
import { enterProject, SIGNED_IN } from './api'

/*
 * Moving a session into another project.
 *
 * The project is in the token, so moving means asking for a new one — and the
 * two things that have to happen alongside it are the ones a caller would
 * forget. The cached role has to come from the answer, because a person is an
 * editor in one project and a viewer in the next. And the sign-in event has to
 * be announced, because that is what empties the query cache: every key in it
 * belongs to the project being left, and `['catalog']` is the key for
 * everybody. Serving one project's catalogue under another's name is the leak
 * this path exists to avoid — and is one this portal has shipped before.
 */

const real = {
  fetch: globalThis.fetch,
  api: process.env.VITE_CRONOS_API,
  storage: Object.getOwnPropertyDescriptor(globalThis, 'localStorage'),
}
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
  process.env.VITE_CRONOS_API = 'https://cronos.example'
  store.set('cronos.token', 'the-old-token')
  store.set('cronos.user', JSON.stringify({
    id: 'u1', email: 'ada@acme.example', org: 'acme', project: 'finance', role: 'editor',
  }))
})

afterEach(() => {
  globalThis.fetch = real.fetch
  if (real.api === undefined) delete process.env.VITE_CRONOS_API
  else process.env.VITE_CRONOS_API = real.api
  if (real.storage) Object.defineProperty(globalThis, 'localStorage', real.storage)
  else Reflect.deleteProperty(globalThis, 'localStorage')
})

function answers(body: unknown, status = 200) {
  const sent: { url: string; init: RequestInit }[] = []
  globalThis.fetch = mock(async (url: unknown, init: RequestInit = {}) => {
    sent.push({ url: String(url), init })
    return new Response(JSON.stringify(body), { status })
  }) as unknown as typeof fetch
  return sent
}

const user = () => JSON.parse(store.get('cronos.user') ?? '{}')

test('the new token replaces the old one, and the project with it', async () => {
  const sent = answers({ token: 'the-new-token', project: 'operations', role: 'viewer' })

  await enterProject('operations')

  expect(sent[0]!.url).toContain('/v1/auth/project')
  expect(sent[0]!.init.method).toBe('POST')
  expect(JSON.parse(String(sent[0]!.init.body))).toEqual({ project: 'operations' })
  expect(store.get('cronos.token')).toBe('the-new-token')
  expect(user().project).toBe('operations')
})

/*
 * The role comes from the answer, not from the project that was left.
 *
 * Carrying the old one over is the mistake that reads as working: an editor
 * moves into a project where they are a viewer and is offered every control
 * they had a moment ago, each of which the server then refuses.
 */
test('the role is the one the new project gives them', async () => {
  answers({ token: 't', project: 'operations', role: 'viewer' })

  await enterProject('operations')

  expect(user().role).toBe('viewer')
})

/*
 * Empty is a real answer, not a missing one: somebody who arrives on their
 * organisation role holds no membership in the project and the server says so
 * by minting a token with no project role. The org role in the session is what
 * decides what they may do there.
 */
test('arriving on an organisation role leaves the project role empty', async () => {
  answers({ token: 't', project: 'operations' })

  await enterProject('operations')

  expect(user().role).toBe('')
  expect(user().org).toBe('acme')
})

test('the session change is announced, which is what empties the cache', async () => {
  answers({ token: 't', project: 'operations', role: 'admin' })
  let announced = 0
  const listen = () => { announced++ }
  globalThis.addEventListener(SIGNED_IN, listen)

  await enterProject('operations')
  globalThis.removeEventListener(SIGNED_IN, listen)

  expect(announced).toBe(1)
})

/*
 * A refusal changes nothing at all.
 *
 * Half a switch is the worst outcome available here: a token for one project
 * beside a cached user naming another, which is a portal showing one project's
 * name over another project's rows.
 */
test('a refused switch leaves the session where it was', async () => {
  answers({ error: 'No such project in this organization.' }, 404)

  await expect(enterProject('somewhere-else')).rejects.toThrow()

  expect(store.get('cronos.token')).toBe('the-old-token')
  expect(user().project).toBe('finance')
  expect(user().role).toBe('editor')
})
