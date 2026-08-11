import { useQuery } from '@tanstack/react-query'
import { connected, runReport, type ReportView, type RunFilters } from './api'

/**
 * A report's data, from the server when there is one.
 *
 * `enabled` is what keeps the sample-data mode working: with no server the
 * query never runs, so a page falls back to its fixture rather than rendering
 * an error nobody can act on. The shell says which mode this is — see
 * SampleBanner — so the fallback is announced rather than disguised.
 */
export function useReport(name: string, filters: RunFilters = {}) {
  const live = connected()

  const query = useQuery<ReportView>({
    queryKey: ['report', name, filters],
    queryFn: () => runReport(name, { filters }),
    enabled: live && name !== '',
    // A report is a query against somebody's warehouse. Re-running it because
    // a window regained focus is a cost their DBA notices and the reader did
    // not ask for.
    refetchOnWindowFocus: false,
    staleTime: 30_000,
    retry: 1,
  })

  return { ...query, live }
}
