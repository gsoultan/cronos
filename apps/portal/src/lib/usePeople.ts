import { useQuery, useQueryClient } from '@tanstack/react-query'
import { connected, invitations, type Invitation, listPeople, type Person } from './api'

/**
 * Who has access to this project.
 *
 * Only against a server, and only for an administrator: the endpoint refuses
 * everybody else, including the list. Who works here, what their addresses are
 * and when each of them last signed in is a description of an organisation.
 */
export function usePeople() {
  const live = connected()
  const queries = useQueryClient()

  const query = useQuery<{ people: Person[]; canInvite: boolean }>({
    queryKey: ['people'],
    queryFn: listPeople,
    enabled: live,
    refetchOnWindowFocus: false,
    // Short, because this is the page somebody is on while they take access
    // away from a person who is standing next to them.
    staleTime: 5_000,
    retry: 1,
  })

  return {
    ...query,
    live,
    /** Re-reads after a change, so the row shows what the server now holds. */
    refresh: () => queries.invalidateQueries({ queryKey: ['people'] }),
  }
}

/**
 * Who has been invited and has not accepted.
 *
 * Its own query rather than a field on the roster: it changes on a different
 * schedule — an invitation expires on its own, with nobody touching the
 * interface — and a stale roster is a cosmetic problem where a stale list of
 * live credentials is not.
 */
export function useInvitations() {
  const query = useQuery<Invitation[]>({
    queryKey: ['invitations'],
    queryFn: invitations,
    enabled: connected(),
    // A minute. Long enough not to be chatty, short enough that one somebody
    // withdrew from another browser does not sit here looking live.
    staleTime: 60_000,
  })
  return { ...query, refresh: () => query.refetch() }
}
