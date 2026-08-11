import { useMemo, useState } from 'react'
import { Button, TextInput } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { Tag } from '../components/StatusPill'
import { Pagination, paginate } from '../components/Pagination'
import { DataSourceForm } from '../forms/DataSourceForm'
import { DatasetForm } from '../forms/DatasetForm'
import { SOURCE_KINDS } from '../lib/sources'
import { connectedSources, listedDatasets } from '../lib/dataSources'
import { relativeTime } from '../lib/format'
import { useWorkspace } from '../lib/WorkspaceContext'
import { canEdit } from '../lib/workspace'

type Panel = 'none' | 'source' | 'dataset'

const PAGE_SIZE = 6
const CARD = 'mb-6 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'
const ROW = 'flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0'

/**
 * Sources and datasets, in that order, because that is the dependency: a source
 * is a connection, a dataset is a governed query against one, and a report
 * binds to a dataset — never to a source directly.
 *
 * One search box across both lists. Someone hunting for "invoices" is looking
 * for a thing, not for a category, and making them guess which of two boxes to
 * type into is a question the interface should answer for them.
 */
export function DataPage() {
  const [panel, setPanel] = useState<Panel>('none')
  const [query, setQuery] = useState('')
  const [sourcePage, setSourcePage] = useState(0)
  const [datasetPage, setDatasetPage] = useState(0)

  const { org, project } = useWorkspace()
  const editable = canEdit(org, project)
  const close = () => setPanel('none')

  /* Searching moves you to a different list, so the page you were on no longer
     means anything. Not resetting it is the classic way to land on an empty
     page three and conclude there are no results. */
  function search(next: string) {
    setQuery(next)
    setSourcePage(0)
    setDatasetPage(0)
  }

  const term = query.trim().toLowerCase()
  const matches = (...fields: string[]) =>
    !term || fields.some((f) => f.toLowerCase().includes(term))

  const sources = useMemo(
    () => connectedSources.filter((s) => matches(s.name, s.detail, s.kind)),
    [term],
  )
  const datasets = useMemo(
    () => listedDatasets.filter((d) => matches(d.label, d.name, d.description, d.source)),
    [term],
  )

  const sourceView = paginate(sources, sourcePage, PAGE_SIZE)
  const datasetView = paginate(datasets, datasetPage, PAGE_SIZE)
  const nothingMatched = term && sources.length === 0 && datasets.length === 0

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

      <div className="mb-6 flex flex-wrap items-center gap-3">
        <TextInput type="search" value={query} className="w-full max-w-[360px]"
          placeholder="Search sources and datasets…" aria-label="Search sources and datasets"
          onChange={(e) => search(e.currentTarget.value)} />
        {term && (
          <span className="text-small text-ink-secondary">
            {sources.length} source{sources.length === 1 ? '' : 's'} ·{' '}
            {datasets.length} dataset{datasets.length === 1 ? '' : 's'}
          </span>
        )}
      </div>

      {nothingMatched ? (
        <EmptyState title={`Nothing matches “${query}”`}
          description="Try a shorter word. Search looks at names, descriptions and connection details."
          action={<Button variant="default" onClick={() => search('')}>Clear search</Button>} />
      ) : (
        <>
          <section className={CARD} data-testid="sources-card">
            <div className={HEAD}>
              <h2 className="text-lead font-semibold text-ink">Sources</h2>
              <span className="text-small text-ink-muted">
                {sources.length} of {connectedSources.length}
              </span>
            </div>
            {sources.length === 0 ? (
              <p className="px-4 py-8 text-center text-small text-ink-muted">
                No sources match “{query}”.
              </p>
            ) : (
              <>
                <ul>
                  {sourceView.slice.map((s) => {
                    const spec = SOURCE_KINDS.find((k) => k.id === s.kind)!
                    return (
                      <li key={s.id} className={ROW}>
                        <span className="text-title" aria-hidden>{spec.icon}</span>
                        <span className="grid min-w-[200px] flex-1 gap-0.5">
                          <span className="font-semibold text-ink">{s.name}</span>
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
                <Pagination page={sourceView.page} pageSize={PAGE_SIZE} total={sources.length}
                  onPage={setSourcePage} noun="sources" />
              </>
            )}
          </section>

          <section className={CARD} data-testid="datasets-card">
            <div className={HEAD}>
              <h2 className="text-lead font-semibold text-ink">Datasets</h2>
              <span className="text-small text-ink-muted">
                {datasets.length} of {listedDatasets.length}
              </span>
            </div>
            {datasets.length === 0 ? (
              <p className="px-4 py-8 text-center text-small text-ink-muted">
                No datasets match “{query}”.
              </p>
            ) : (
              <>
                <ul>
                  {datasetView.slice.map((d) => (
                    <li key={d.id} className={ROW}>
                      <span className="grid min-w-[220px] flex-1 gap-0.5">
                        <span className="font-semibold text-ink">{d.label}</span>
                        <span className="text-small text-ink-secondary">{d.description}</span>
                      </span>
                      <span className="flex gap-1">
                        <Tag>{d.fields} fields</Tag>
                        {d.measures && <Tag>summarisable</Tag>}
                      </span>
                      <span className="text-small text-ink-secondary">{d.source}</span>
                      <span className="text-caption text-ink-muted">
                        {relativeTime(d.updatedAt)}
                      </span>
                    </li>
                  ))}
                </ul>
                <Pagination page={datasetView.page} pageSize={PAGE_SIZE} total={datasets.length}
                  onPage={setDatasetPage} noun="datasets" />
              </>
            )}
          </section>
        </>
      )}
    </>
  )
}
