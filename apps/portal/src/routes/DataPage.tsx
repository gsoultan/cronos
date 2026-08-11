import { useState } from 'react'
import { Button } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { Tag } from '../components/StatusPill'
import { DataSourceForm } from '../forms/DataSourceForm'
import { DatasetForm } from '../forms/DatasetForm'
import { SOURCE_KINDS } from '../lib/sources'
import { datasets } from '../lib/mock'
import { useWorkspace } from '../lib/WorkspaceContext'
import { canEdit } from '../lib/workspace'

type Panel = 'none' | 'source' | 'dataset'

const CONNECTED = [
  { name: 'Production warehouse', kind: 'postgres', detail: 'db.internal.acme.com · 24 tables', ok: true, datasets: 2 },
  { name: 'Event lake', kind: 'objectstore', detail: 's3://acme-lake/events/ · parquet', ok: true, datasets: 0 },
  { name: 'Billing API', kind: 'api', detail: 'api.billing.acme.com · cached 5 min', ok: false, datasets: 0 },
]

const ROW = 'mb-2 flex items-center gap-4 rounded-lg border border-line bg-surface p-4'

/**
 * Sources and datasets, in that order, because that is the dependency:
 * a source is a connection, a dataset is a governed query against one, and a
 * report binds to a dataset — never to a source directly.
 */
export function DataPage() {
  const [panel, setPanel] = useState<Panel>('none')
  const { org, project } = useWorkspace()
  const editable = canEdit(org, project)
  const close = () => setPanel('none')

  if (panel === 'source') {
    return (
      <>
        <PageHeader title="Connect a source"
          description="Four short steps. Nothing is saved until the connection works." />
        <DataSourceForm onDone={close} onCancel={close} />
      </>
    )
  }

  if (panel === 'dataset') {
    return (
      <>
        <PageHeader title="New dataset"
          description="A query against one source, plus the fields and rules every report using it inherits." />
        <DatasetForm onDone={close} onCancel={close} />
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Data"
        description="Sources are connections. Datasets are governed queries against them — reports bind to datasets, never to a source directly."
        actions={editable ? (
          <>
            <Button variant="default" onClick={() => setPanel('source')}>Connect a source</Button>
            <Button onClick={() => setPanel('dataset')}>New dataset</Button>
          </>
        ) : undefined}
      />

      <section className="mb-8">
        <h2 className="mb-3 text-caption font-semibold tracking-[0.06em] text-ink-muted uppercase">
          Sources
        </h2>
        {CONNECTED.length === 0 ? (
          <EmptyState title="No sources yet"
            description="Connect a database, a bucket of files, a spreadsheet or an API. It takes about a minute."
            action={<Button onClick={() => setPanel('source')}>Connect a source</Button>} />
        ) : (
          <ul>
            {CONNECTED.map((s) => {
              const spec = SOURCE_KINDS.find((k) => k.id === s.kind)!
              return (
                <li key={s.name} className={ROW}>
                  <span className="text-title" aria-hidden>{spec.icon}</span>
                  <span className="grid min-w-0 flex-1 gap-0.5">
                    <span className="font-semibold">{s.name}</span>
                    <span className="text-small text-ink-secondary">{s.detail}</span>
                  </span>
                  <span className="text-small text-ink-secondary">
                    {s.datasets} dataset{s.datasets === 1 ? '' : 's'}
                  </span>
                  <span className={`text-micro font-medium tracking-[0.04em] uppercase ${
                    spec.pushdown === 'full' ? 'text-delta-good'
                      : spec.pushdown === 'partial' ? 'text-ink-secondary' : 'text-serious'}`}>
                    {spec.pushdownLabel}
                  </span>
                  <span className={`rounded-full px-2 py-px text-micro font-medium ${
                    s.ok ? 'bg-good/15 text-delta-good' : 'bg-serious/20 text-ink'}`}>
                    {s.ok ? 'Connected' : 'Last check failed'}
                  </span>
                </li>
              )
            })}
          </ul>
        )}
      </section>

      <section>
        <h2 className="mb-3 text-caption font-semibold tracking-[0.06em] text-ink-muted uppercase">
          Datasets
        </h2>
        {datasets.length === 0 ? (
          <EmptyState title="No datasets yet"
            description="A dataset turns a query into something report authors can use without writing SQL."
            action={editable
              ? <Button onClick={() => setPanel('dataset')}>New dataset</Button>
              : undefined} />
        ) : (
          <ul>
            {datasets.map((d) => (
              <li key={d.name} className={ROW}>
                <span className="grid min-w-0 flex-1 gap-0.5">
                  <span className="font-semibold">{d.label}</span>
                  <span className="text-small text-ink-secondary">{d.description}</span>
                </span>
                <span className="flex gap-1">
                  <Tag>{d.fields.filter((f) => !f.hidden).length} fields</Tag>
                  {d.fields.some((f) => f.role === 'measure') && <Tag>summarisable</Tag>}
                </span>
                <span className="text-small text-ink-secondary">Production warehouse</span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  )
}
