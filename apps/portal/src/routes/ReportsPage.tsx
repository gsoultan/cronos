import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Button, TextInput } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { Tag } from '../components/StatusPill'
import { reports } from '../lib/mock'
import { useWorkspace } from '../lib/WorkspaceContext'
import { canEdit, reportsIn } from '../lib/workspace'
import { relativeTime } from '../lib/format'
import { useCatalog } from '../lib/useCatalog'

/**
 * The repository. Grouped by folder, because that is how people filed them.
 *
 * This page had no live path at all — it read the sample directory and filtered
 * it by the current project, and nothing else. Connected to a real server it
 * showed "No reports yet" beside however many the server was serving, because
 * the real project matches no fixture.
 *
 * It survived because the development script never connected the portal to its
 * own API, so the only view anybody had was sample mode, where the fixture
 * project matches the fixture reports and everything looks right.
 */
export function ReportsPage() {
  const [q, setQ] = useState('')
  const { org, project } = useWorkspace()
  const catalog = useCatalog()

  /*
   * The server's reports, in the shape the fixture uses.
   *
   * Mapped rather than branched on throughout: the grouping, the search and the
   * cards below are the same work either way, and two copies of them would be
   * two places for the live one to fall behind.
   *
   * The fields a server does not have are left out rather than invented. There
   * is no "edited by" in a catalogue — the definition store knows, and this
   * endpoint does not ask — and a fabricated name under every report is exactly
   * the class of thing this page has just stopped doing.
   */
  const live = useMemo<Card[]>(() => (catalog.data?.reports ?? []).map((r) => ({
    name: r.name,
    label: r.title ?? r.name,
    description: r.description ?? '',
    folder: r.folder ?? 'Reports',
    outputs: r.outputs,
    scheduled: undefined,
    updatedAt: undefined,
    updatedBy: undefined,
  })), [catalog.data])

  /* Reports are resolved within the active project. Nothing here can name a
     report from another project — that is resource ownership, not a filter. */
  const visible = useMemo<Card[]>(
    () => (catalog.live ? live : reportsIn(reports, project)),
    [catalog.live, live, project])

  const folders = useMemo(() => {
    const term = q.trim().toLowerCase()
    const matched = visible.filter((r) =>
      !term || r.label.toLowerCase().includes(term) || r.folder.toLowerCase().includes(term))
    const byFolder = new Map<string, typeof visible>()
    for (const r of matched) {
      const list = byFolder.get(r.folder) ?? []
      list.push(r)
      byFolder.set(r.folder, list)
    }
    return [...byFolder.entries()].toSorted(([a], [b]) => a.localeCompare(b))
  }, [q, visible])

  const editable = canEdit(org, project)
  const newReport = <Button component={Link} to="/reports/new">New report</Button>

  const header = (
    <PageHeader
      title="Reports"
      description="Dashboards, statements and exports — all reports, differing only in what they output."
      actions={editable ? newReport : undefined}
    />
  )

  /*
    A project that could not be read is not a project with no reports.

    This page said "No reports in finance yet" whenever the catalogue query
    failed, because a failed query and an empty one both arrive here as an
    empty list — and then invited somebody to build their first report, with a
    button leading to a builder that could not reach the server either.

    Which is the worst answer available. Somebody whose API is down for a deploy
    opens this page to check on their reports and is told they have none. Data
    and Activity have always said "could not read"; this page and Schedules said
    nothing was there.

    Checked before pending, deliberately: a query that has failed and is
    retrying is both, and a spinner over a server that is not coming back is
    something nobody can act on.
  */
  if (catalog.live && catalog.error) {
    return (
      <>
        {header}
        <EmptyState title="Could not read this project"
          description={catalog.error instanceof Error
            ? catalog.error.message
            : 'The server did not answer. Nothing has been changed.'} />
      </>
    )
  }
  if (catalog.live && catalog.isPending) {
    return (
      <>
        {header}
        <p className="p-8 text-center text-ink-muted">Loading…</p>
      </>
    )
  }

  return (
    <>
      {header}

      {visible.length > 0 && (
        <TextInput placeholder="Search reports…" value={q} aria-label="Search reports"
          onChange={(e) => setQ(e.currentTarget.value)} className="mb-6 max-w-[360px]" />
      )}

      {visible.length === 0 ? (
        <EmptyState
          title={`No reports in ${project.name} yet`}
          description={editable
            ? 'Connect a data source, then build your first report — it takes about five minutes.'
            : 'Nobody has built a report here yet. Ask a project editor to add one.'}
          action={editable ? newReport : undefined}
        />
      ) : folders.length === 0 ? (
        <EmptyState
          title="Nothing matches that search"
          description="Try a shorter word, or clear the search to see everything in this project."
          action={<Button variant="default" onClick={() => setQ('')}>Clear search</Button>}
        />
      ) : (
        folders.map(([folder, items]) => (
          <section key={folder} className="mb-8">
            <h2 className="mb-3 text-caption font-semibold tracking-[0.06em] text-ink-muted uppercase">
              {folder}
            </h2>
            <ul className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(280px,1fr))]">
              {items.map((r) => (
                <li key={r.name}>
                  <Link to="/reports/$name" params={{ name: r.name }}
                    className="flex h-full flex-col gap-2 rounded-lg border border-line
                               bg-surface p-4 text-ink no-underline shadow-card transition
                               duration-150 ease-out-quick hover:-translate-y-px hover:border-accent">
                    <span className="font-semibold">{r.label}</span>
                    <span className="flex-1 text-small text-ink-secondary">{r.description}</span>
                    <span className="flex flex-wrap gap-1">
                      {r.scheduled && (
                        <Tag accent>Sends to {r.scheduled.recipients.toLocaleString('en')}</Tag>
                      )}
                      {r.outputs.includes('pdf') && <Tag>PDF</Tag>}
                      {r.outputs.includes('xlsx') && <Tag>Excel</Tag>}
                    </span>
                    {/* Only where it is known. A catalogue does not carry who
                        last edited a definition, and a name invented under
                        every card is the thing this page has just stopped
                        doing. */}
                    {r.updatedAt && (
                      <span className="border-t border-line pt-2 text-caption text-ink-muted">
                        Edited {relativeTime(r.updatedAt)}
                        {r.updatedBy ? ` by ${r.updatedBy}` : ''}
                      </span>
                    )}
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        ))
      )}
    </>
  )
}

/**
 * One card, from either source.
 *
 * The fields a catalogue cannot answer are optional rather than filled in with
 * something plausible. "Edited two days ago by Dewi" under a report the server
 * has never heard of is the same class of thing as a device list invented in the
 * browser, and it reads as true for exactly as long as nobody checks.
 */
interface Card {
  name: string
  label: string
  description?: string
  folder: string
  outputs: string[]
  scheduled?: { recipients: number }
  updatedAt?: string
  updatedBy?: string
}
