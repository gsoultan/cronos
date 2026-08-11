import { useQuery } from '@tanstack/react-query'
import { connected, readDefinition } from './api'
import type { Loaded } from './definitions'

/**
 * An existing definition, read back into the shape a form holds.
 *
 * Editing is publishing the same name again — the store upserts and keeps the
 * previous bytes addressable — so the only new machinery an edit path needs is
 * this: fetch the author's exact document and hand it to the reader.
 *
 * The reader also reports what the form cannot show, and that travels with the
 * value rather than beside it, because the two are only ever useful together.
 *
 * Unconnected there is nothing to load, and `name` being absent means the page
 * is creating rather than editing. Both answer `undefined`, which is what a
 * form reads as "start empty".
 */
export function useDefinition<T>(
  kind: string,
  name: string | undefined,
  read: (text: string) => Loaded<T>,
) {
  const live = connected()

  const query = useQuery<Loaded<T>>({
    queryKey: ['definition', kind, name],
    queryFn: () => readDefinition(kind, name ?? '').then(read),
    enabled: live && !!name,
    refetchOnWindowFocus: false,
    // Never stale while a form is open. A refetch behind an author mid-edit
    // would either be ignored or overwrite what they had typed, and neither is
    // something they asked for.
    staleTime: Infinity,
    retry: 1,
  })

  return {
    initial: query.data,
    /* Editing something that is not here yet is not the same as editing
       nothing: a form that renders empty would offer to create a definition at
       a name that already exists somewhere the reader could not reach. */
    pending: !!name && live && query.isPending,
    error: query.error ? `Could not load ${kind} ${name}.` : null,
    editing: !!name,
  }
}
