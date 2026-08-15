import { QueryClient } from '@tanstack/react-query'
import { SIGNED_IN, SIGNED_OUT } from './api'

/**
 * One query cache, and it belongs to a session.
 *
 * Every key here names what was asked for and none of them name who asked:
 * `['catalog']`, `['runs', limit]`, `['people']`. That is fine while a page
 * load has one session and wrong the moment it has two. Sign out, sign in as
 * somebody in another organisation, and the second person is served the first
 * one's catalogue out of cache — under their own organisation in the header,
 * which is what makes it convincing. cronos is embedded in other people's
 * products, so those two sessions are routinely two different customers.
 *
 * Naming the tenant in every key would also work, and would be the worse fix:
 * it spreads the guarantee across every hook, including the ones nobody has
 * written yet, and the first one that forgets is the leak. A cache is session
 * state. It ends when the session ends, in one place, and no hook has to know.
 *
 * Cleared on the way in as well as the way out, because a token can be replaced
 * without a sign-out — an SSO callback adopts one from the URL fragment — and
 * that path puts the same two sessions in one page load.
 */
export const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, refetchOnWindowFocus: false } },
})

/** Ties a cache to the session. Returns the undo, which is for tests. */
export function forgetOnSessionChange(client: QueryClient): () => void {
  const forget = () => client.clear()
  globalThis.addEventListener?.(SIGNED_IN, forget)
  globalThis.addEventListener?.(SIGNED_OUT, forget)
  return () => {
    globalThis.removeEventListener?.(SIGNED_IN, forget)
    globalThis.removeEventListener?.(SIGNED_OUT, forget)
  }
}

forgetOnSessionChange(queryClient)
