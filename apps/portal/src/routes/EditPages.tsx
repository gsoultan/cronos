import type { ReactNode } from 'react'
import { useNavigate, useParams } from '@tanstack/react-router'
import { Button } from '@mantine/core'
import { EmptyState } from '../components/EmptyState'
import { PageHeader } from '../components/PageHeader'
import { DataSourceForm } from '../forms/DataSourceForm'
import { DatasetForm } from '../forms/DatasetForm'
import { ReportForm } from '../forms/ReportForm'
import { ScheduleForm } from '../forms/ScheduleForm'
import {
  readDataset, readDataSource, readReport, readSchedule, type Loaded,
} from '../lib/definitions'
import { useDefinition } from '../lib/useDefinition'

/**
 * Editing a definition.
 *
 * The same four forms, given a document to start from. Nothing about saving
 * changes: publishing a name that already exists replaces it and keeps the old
 * bytes addressable, so an edit is a create with the fields filled in.
 *
 * The form does not mount until the document has arrived. Seeding form state
 * after mount would mean either an effect that overwrites what somebody has
 * already typed, or a `key` remount that throws it away — and there is nothing
 * useful to show in the meantime anyway.
 */
function Edit<T>({ kind, read, children }: {
  kind: string
  read: (text: string) => Loaded<T>
  children: (initial: Loaded<T> | undefined, done: () => void) => ReactNode
}) {
  const { name } = useParams({ strict: false })
  const navigate = useNavigate()
  const { initial, pending, error } = useDefinition(kind, name, read)

  const done = () => void navigate({ to: backTo(kind) })

  if (pending) {
    return <p data-testid="edit-loading" className="p-8 text-center text-ink-muted">Loading…</p>
  }
  if (error) {
    return (
      <EmptyState title={`That ${kind.toLowerCase()} could not be loaded`}
        description={`${error} It may have been deleted, or renamed by someone else.`}
        action={<Button onClick={done}>Go back</Button>} />
    )
  }
  return <>{children(initial, done)}</>
}

/** Where cancelling lands: the list the definition came from. */
function backTo(kind: string): string {
  if (kind === 'Report') return '/'
  if (kind === 'Schedule') return '/schedules'
  return '/data'
}

/* One route component per kind. They differ only in the reader and the form,
   and that difference is the entire content of each. */

export function EditReportPage() {
  return (
    <Edit kind="Report" read={readReport}>
      {(initial, done) => <ReportForm initial={initial} onDone={done} onCancel={done} />}
    </Edit>
  )
}

export function EditDatasetPage() {
  return (
    <Edit kind="Dataset" read={readDataset}>
      {(initial, done) => (
        <div className="mx-auto max-w-[1400px] p-6">
          <PageHeader title={initial?.input.name || 'Dataset'}
            description="Changes replace the stored definition. The previous version stays addressable." />
          <DatasetForm initial={initial} onDone={done} onCancel={done} />
        </div>
      )}
    </Edit>
  )
}

export function EditDataSourcePage() {
  return (
    <Edit kind="DataSource" read={readDataSource}>
      {(initial, done) => (
        <div className="mx-auto max-w-[1400px] p-6">
          <PageHeader title={initial?.input.name || 'Data source'}
            description="The password is not shown. Leaving it blank keeps the one already in use." />
          <DataSourceForm initial={initial} onDone={done} onCancel={done} />
        </div>
      )}
    </Edit>
  )
}

export function EditSchedulePage() {
  return (
    <Edit kind="Schedule" read={readSchedule}>
      {(initial, done) => (
        <div className="mx-auto max-w-[900px] p-6">
          <PageHeader title={initial?.input.name || 'Schedule'}
            description="Changes replace the stored definition. The previous version stays addressable." />
          <ScheduleForm initial={initial} onDone={done} onCancel={done} />
        </div>
      )}
    </Edit>
  )
}
