import { useQuery } from '@tanstack/react-query'
import { connected, listRuns, readRun, type Run, type RunDelivery } from './api'

/**
 * What happened, and to whom.
 *
 * The one question a scheduler creates and nothing else answers. A report you
 * open is a report you can see is wrong; a report mailed to 812 customers at
 * 06:00 on the first is one you find out about from a support ticket, unless
 * something here says which of them it reached.
 *
 * Only against a server, because only a server ran anything. Sample mode has
 * no history to invent — a fixture of plausible deliveries would be a page
 * that looks like an audit and is not one.
 */
export function useRuns(limit = 50) {
  const live = connected()

  const query = useQuery<{ runs: Run[] }>({
    queryKey: ['runs', limit],
    queryFn: () => listRuns(limit),
    enabled: live,
    refetchOnWindowFocus: false,
    // A run in progress finishes while somebody is looking at it, and the
    // difference between "sending" and "sent to 812" is the whole point of
    // being on this page.
    refetchInterval: (q) => (q.state.data?.runs.some(sending) ? 5_000 : false),
    staleTime: 10_000,
    retry: 1,
  })

  return { ...query, live }
}

function sending(r: Run): boolean {
  return r.status === 'running'
}

/** One run, and every delivery it attempted. */
export function useRun(id: string | undefined) {
  const live = connected()

  const query = useQuery<{ run: Run; deliveries: RunDelivery[] }>({
    queryKey: ['run', id],
    queryFn: () => readRun(id ?? ''),
    enabled: live && !!id,
    refetchOnWindowFocus: false,
    // A burst writes its deliveries as they happen, so a list opened while one
    // is running is a list that is still filling. Without this it would show
    // whatever had been written the moment somebody clicked and never say
    // another word — which reads as "these are the recipients" rather than
    // "these are the recipients so far".
    refetchInterval: (q) => (q.state.data?.run.status === 'running' ? 2_000 : false),
    retry: 1,
  })

  return { ...query, live }
}
