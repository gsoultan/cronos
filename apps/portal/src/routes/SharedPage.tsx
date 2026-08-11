import { useQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { EmptyState } from '../components/EmptyState'
import { LiveReport } from '../components/LiveReport'
import { openShare, readShared, type ReportView } from '../lib/api'

/**
 * A report opened by a link, by somebody who is not in the project.
 *
 * No shell, no navigation, no sign-in. Whoever follows a share link has no
 * identity here and no other business in the portal: showing them a rail of
 * pages they cannot open would be an invitation to try.
 *
 * The link is the credential, and it is not the token. Opening exchanges the
 * id for a token pinned to one report, which lives twenty-four hours at most —
 * so a URL forwarded around a company is not a permanent key, and withdrawing
 * the share stops it on the next request rather than at the next expiry.
 */
export function SharedPage() {
  const { id } = useParams({ strict: false })

  const query = useQuery<ReportView>({
    queryKey: ['shared', id],
    queryFn: async () => {
      const { token, report } = await openShare(id ?? '')
      return readShared(token, report)
    },
    enabled: !!id,
    refetchOnWindowFocus: false,
    // One attempt. A link that does not open will not open on the second try,
    // and retrying a 404 three times is three chances to look broken.
    retry: false,
  })

  return (
    <main className="mx-auto max-w-[1100px] p-6">
      {query.isPending && (
        <p data-testid="shared-loading" className="p-8 text-center text-ink-muted">Loading…</p>
      )}

      {query.error && (
        /* One message for a link that never existed, one that expired and one
           somebody withdrew — the server does not distinguish them either, and
           telling them apart would let whoever holds a dead link learn that a
           live one had been there. */
        <EmptyState title="This link does not open"
          description="It may have expired, or the person who shared it may have withdrawn it. Ask them for a new one." />
      )}

      {query.data && (
        <>
          <header className="mb-6">
            <h1 className="text-title font-semibold text-ink">{query.data.title}</h1>
            {query.data.description && (
              <p className="mt-1 text-ink-secondary">{query.data.description}</p>
            )}
            {/* Said plainly. Somebody reading a shared report should know it is
                one — that they are seeing it through somebody else's access,
                and that the numbers are current rather than a snapshot. */}
            <p className="mt-2 text-caption text-ink-muted" data-testid="shared-notice">
              Shared with you. This shows current data, as the person who shared it sees it.
            </p>
          </header>
          <LiveReport view={query.data} />
        </>
      )}
    </main>
  )
}
