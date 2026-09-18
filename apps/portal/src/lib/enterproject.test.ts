import { afterEach, beforeEach, expect, test } from 'bun:test'
import { enterableProjects, enterProject, SIGNED_IN } from './api'

/* Switching project is the server minting a new session, not the client
   editing the one it has. These cover what the client must do with what comes
   back — because a token replaced badly leaves a session naming the project
   somebody just left, and every request after it is answered for the wrong
   place. */

const realStore = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')
const realFetch = globalThis.fetch
let store: Map<string, string>

beforeEach(() => {
  store = new Map<string, string>([
    ['cronos.token', 'old-token'],
    ['cronos.user', JSON.stringify({ id: 'u1', email: 'dewi@acme.example', org: 'acme', project: 'finance', role: 'admin' })],
  ])
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true, writable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
    },
  })
  import.meta.env.VITE_CRONOS_API = 'http://cronos.test'
})

afterEach(() => {
  if (realStore) Object.defineProperty(globalThis, 'localStorage', realStore)
  globalThis.fetch = realFetch
})

function answer(status: number, body: unknown) {
  globalThis.fetch = (async () => new Response(JSON.stringify(body), {
    status, headers: { 'content-type': 'application/json' },
  })) as unknown as typeof fetch
}

test('entering a project replaces the session it was given', async () => {
  answer(200, { token: 'new-token', project: 'ops' })
  await enterProject('ops')
  expect(store.get('cronos.token')).toBe('new-token')
})

/* The stored user names a project too, and the shell reads it back. Left alone
   it would say finance while the token says ops, and the screen and the
   requests would disagree — with the screen wrong. */
test('and the project the shell reads back', async () => {
  answer(200, { token: 'new-token', project: 'ops' })
  await enterProject('ops')
  expect(JSON.parse(store.get('cronos.user')!).project).toBe('ops')
  expect(JSON.parse(store.get('cronos.user')!).email).toBe('dewi@acme.example')
})

test('and says so, so everything hanging off the session refetches', async () => {
  answer(200, { token: 'new-token', project: 'ops' })
  let fired = 0
  const on = () => { fired++ }
  globalThis.addEventListener(SIGNED_IN, on)
  await enterProject('ops')
  globalThis.removeEventListener(SIGNED_IN, on)
  expect(fired).toBe(1)
})

/* A refusal must not touch the session. Half-switching — new project, old
   token, or the reverse — is worse than not switching. */
test('a refusal leaves the session alone', async () => {
  answer(404, { error: 'No such project you can enter.' })
  await expect(enterProject('secret')).rejects.toThrow()
  expect(store.get('cronos.token')).toBe('old-token')
  expect(JSON.parse(store.get('cronos.user')!).project).toBe('finance')
})

/* A 200 with nothing in it is a server that did not do what it said. Keeping
   the old token is the safe half of that. */
test('and so does a reply with no token in it', async () => {
  answer(200, { project: 'ops' })
  await expect(enterProject('ops')).rejects.toThrow()
  expect(store.get('cronos.token')).toBe('old-token')
})

/* The route is not mounted on a deployment with no records store. That is not
   an error, it is a deployment with one project. */
test('a build that cannot switch offers nothing rather than failing', async () => {
  answer(404, { error: 'not found' })
  expect(await enterableProjects()).toEqual([])
})

test('and otherwise offers what the server listed', async () => {
  answer(200, { projects: [{ project: 'finance', role: 'admin', via: 'membership' }] })
  const got = await enterableProjects()
  expect(got).toHaveLength(1)
  expect(got[0]!.project).toBe('finance')
  expect(got[0]!.via).toBe('membership')
})
