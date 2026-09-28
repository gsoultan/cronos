import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, connected, secrets } from './api'
import type { SecretList } from './types'

/**
 * The project's secrets: names, where each value comes from, and what uses it.
 *
 * One key, `['secrets']`, shared by the three places that ask — the Settings
 * tab, the datasource wizard and a map's basemap — so a key set from the
 * builder shows as stored on the Settings tab without either knowing about the
 * other. It names no tenant, like every key here; the cache is emptied when
 * the session changes (lib/queryClient.ts), which is what makes that safe.
 *
 * Unconnected it never runs. Sample mode has no server to keep a secret, and
 * the pages that ask say so rather than inventing a list.
 */
export function useSecrets(wanted = true) {
  const live = connected()
  const queries = useQueryClient()

  const query = useQuery<SecretList>({
    queryKey: ['secrets'],
    queryFn: secrets,
    enabled: live && wanted,
    refetchOnWindowFocus: false,
    // Short: this is the page somebody is on while they fix a broken map, and
    // an answer from before the fix reads as a fix that did not work.
    staleTime: 5_000,
    // A refusal or a missing endpoint is an answer, and asking again gets the
    // same one. Only a server that did not answer is worth a second try.
    retry: (failures, err) =>
      failures < 1 && !(err instanceof ApiError && err.status >= 400 && err.status < 500),
  })

  return {
    ...query,
    live,
    storing: storing(query.data, query.error),
    /** Re-reads after a change, so a row shows what the server now holds. */
    refresh: () => queries.invalidateQueries({ queryKey: ['secrets'] }),
  }
}

/**
 * Whether this deployment can keep a secret: yes, no, or not known yet.
 *
 * No endpoint at all is a no. It is mounted wherever there is a definition
 * store, so its absence is a file-backed deployment — or one older than stored
 * secrets — and either way the only place a value can live is the server's
 * environment. Undefined while loading or after any other failure, which the
 * forms treat as "offer to store it and let the server answer", rather than
 * as a refusal nobody gave.
 */
function storing(data: SecretList | undefined, error: unknown): boolean | undefined {
  if (data) return data.store
  if (error instanceof ApiError && error.status === 404) return false
  return undefined
}
