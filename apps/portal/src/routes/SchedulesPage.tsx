import { useState } from 'react'
import { Button } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { ScheduleForm } from '../forms/ScheduleForm'
import { cronToText } from '../lib/cronText'
import { relativeTime } from '../lib/format'
import { useWorkspace } from '../lib/WorkspaceContext'
import { canEdit } from '../lib/workspace'

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
  const [adding, setAdding] = useState(false)
  const { org, project } = useWorkspace()
  const editable = canEdit(org, project)

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
