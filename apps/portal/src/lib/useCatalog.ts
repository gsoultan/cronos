import { useQuery } from '@tanstack/react-query'
import { connected, readCatalog, type CatalogView } from './api'

/**
 * What the project contains, from the server when there is one.
 *
 * One query for the whole navigation. `enabled` is what keeps sample mode
 * working: with no server it never runs and a page falls back to its fixture,
 * which the shell announces rather than disguises.
 */
export function useCatalog() {
  const live = connected()

  const query = useQuery<CatalogView>({
    queryKey: ['catalog'],
    queryFn: readCatalog,
    enabled: live,
    refetchOnWindowFocus: false,
    staleTime: 60_000,
    retry: 1,
  })

  return { ...query, live }
}
