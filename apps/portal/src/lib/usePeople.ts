import { useQuery, useQueryClient } from '@tanstack/react-query'
import { connected, listPeople, type Person } from './api'

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

  const query = useQuery<{ people: Person[] }>({
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
