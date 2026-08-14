import { useState } from 'react'
import { Button } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { ScheduleForm } from '../forms/ScheduleForm'
import { cronToText } from '../lib/cronText'
import { relativeTime } from '../lib/format'
import { useWorkspace } from '../lib/WorkspaceContext'
import { canEdit } from '../lib/workspace'
import { useCatalog } from '../lib/useCatalog'
import { LiveSchedules } from '../components/LiveSchedules'

const SCHEDULES = [
  {
    name: 'Monthly customer statements', report: 'Monthly invoice statement',
    cron: '0 6 1 * *', tz: 'Europe/Berlin', recipients: 812,
    lastRun: '2026-08-01T06:00:00Z', lastResult: 'ok' as const, failed: 0,
  },
  {
    name: 'Weekly carrier summary', report: 'Carrier performance',
    cron: '0 7 * * 1', tz: 'Europe/Berlin', recipients: 14,
    lastRun: '2026-08-10T07:00:00Z', lastResult: 'partial' as const, failed: 2,
  },
]

export function SchedulesPage() {
  const catalog = useCatalog()
  const [adding, setAdding] = useState(false)
  const { org, project } = useWorkspace()
  const editable = canEdit(org, project)

  if (catalog.live && !adding) {
    /*
      A project that could not be read is not a project with nothing scheduled.

      The same fault the Reports page had: the query failing and the project
      being empty both arrive as an empty list, so a deploy with the API down
      told somebody nothing was scheduled — on the page they opened to find out
      whether tomorrow's statements would go.

      Before the pending branch, because a query that has failed and is
      retrying is both, and a spinner over a server that is not coming back is
      not something anybody can act on.
    */
    if (catalog.error) {
      return (
        <>
          <PageHeader title="Schedules"
            description="What goes out, when, and to whom." />
          <EmptyState title="Could not read this project"
            description={catalog.error instanceof Error
              ? catalog.error.message
              : 'The server did not answer. Nothing has been changed.'} />
        </>
      )
    }
    if (catalog.isPending) {
      return <p className="p-8 text-center text-ink-muted">Loading…</p>
    }
    return (
      <>
        <PageHeader title="Schedules"
          description="What goes out, when, and to whom." />
        <LiveSchedules schedules={catalog.data?.schedules ?? []} />
      </>
    )
  }

  if (adding) {
    return (
      <>
        <PageHeader eyebrow={`${org.name} · ${project.name}`} title="New schedule"
          description="What goes out, when, and to whom." />
        <ScheduleForm onDone={() => setAdding(false)} onCancel={() => setAdding(false)} />
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Schedules"
        description="What sends automatically, and what happened last time."
        actions={editable ? <Button onClick={() => setAdding(true)}>New schedule</Button> : undefined}
      />

      {SCHEDULES.length === 0 ? (
        <EmptyState title="Nothing is scheduled"
          description="Pick a report and cronos will send it on a schedule — to one address, or a personalised copy to every customer."
          action={<Button onClick={() => setAdding(true)}>New schedule</Button>} />
      ) : (
        <ul>
          {SCHEDULES.map((s) => (
            <li key={s.name} className="mb-2 flex items-center gap-4 rounded-lg border border-line bg-surface p-4">
              <span className="grid min-w-0 flex-1 gap-0.5">
                <span className="font-semibold">{s.name}</span>
                <span className="text-small text-ink-secondary">
                  {s.report} · {cronToText(s.cron)} · {s.tz}
                </span>
              </span>
              <span className="text-small text-ink-secondary">
                {s.recipients.toLocaleString('en')} recipients
              </span>
              <span className="grid justify-items-end gap-0.5">
                {/* A partial failure is its own state: the successes went out,
                    and only the failures need attention. */}
                <span className={`rounded-full px-2 py-px text-micro font-medium ${
                  s.lastResult === 'ok' ? 'bg-good/15 text-delta-good' : 'bg-serious/20 text-ink'}`}>
                  {s.lastResult === 'ok' ? 'Delivered' : `${s.failed} failed`}
                </span>
                <span className="text-caption text-ink-muted">{relativeTime(s.lastRun)}</span>
              </span>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}
