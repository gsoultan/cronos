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

/** The repository. Grouped by folder, because that is how people filed them. */
export function ReportsPage() {
  const [q, setQ] = useState('')
  const { org, project } = useWorkspace()

  /* Reports are resolved within the active project. Nothing here can name a
     report from another project — that is resource ownership, not a filter. */
  const visible = useMemo(() => reportsIn(reports, project), [project])

  const folders = useMemo(() => {
    const term = q.trim().toLowerCase()
    const matched = visible.filter((r) =>
      !term || r.label.toLowerCase().includes(term) || r.folder.toLowerCase().includes(term))
    const byFolder = new Map<string, typeof reports>()
    for (const r of matched) {
      const list = byFolder.get(r.folder) ?? []
      list.push(r)
      byFolder.set(r.folder, list)
    }
    return [...byFolder.entries()].toSorted(([a], [b]) => a.localeCompare(b))
  }, [q, visible])

  const editable = canEdit(org, project)
  const newReport = <Button component={Link} to="/reports/new">New report</Button>

  return (
    <>
      <PageHeader
        title="Reports"
        description="Everything in this project. Open one to run it."
        actions={editable ? newReport : undefined}
      />

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
                    <span className="border-t border-line pt-2 text-caption text-ink-muted">
                      Edited {relativeTime(r.updatedAt)} by {r.updatedBy}
                    </span>
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
