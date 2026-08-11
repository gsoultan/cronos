import { Tag } from './StatusPill'
import { Pagination, paginate } from './Pagination'
import type { DatasetSummary, SourceSummary } from '../lib/api'

const CARD = 'mb-6 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'
const ROW = 'flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0'

/**
 * Sources and datasets as the server has them.
 *
 * The same two-card shape as the sample view, because a page that rearranges
 * itself when it connects teaches somebody the layout twice.
 */
export function LiveSources({ sources, page, size, onPage }: {
  sources: SourceSummary[]
  page: number
  size: number
  onPage: (n: number) => void
}) {
  const view = paginate(sources, page, size)

  return (
    <section className={CARD} data-testid="sources-card">
      <div className={HEAD}>
        <h2 className="text-lead font-semibold text-ink">Sources</h2>
        <span className="text-small text-ink-muted">{sources.length}</span>
      </div>
      {sources.length === 0 ? (
        <p className="px-4 py-8 text-center text-small text-ink-muted">
          No sources are defined. Datasets fall back to the configured database.
        </p>
      ) : (
        <>
          <ul>
            {view.slice.map((s) => (
              <li key={s.name} className={ROW}>
                <span className="grid min-w-[200px] flex-1 gap-0.5">
                  <span className="font-semibold text-ink">{s.name}</span>
                  <span className="text-small text-ink-secondary">
                    {s.description ?? s.detail}
                  </span>
                </span>
                <span className="text-small text-ink-secondary">
                  {s.datasets} dataset{s.datasets === 1 ? '' : 's'}
                </span>
                {/* The limits are the interesting part of a connection and the
                    part nobody reads the file for: what this source will spend
                    on one query, and how much it will hand back. */}
                <span className="text-caption text-ink-muted">
                  {s.timeout} · {s.maxRows.toLocaleString('en')} rows
                </span>
                <Tag>{s.driver}</Tag>
              </li>
            ))}
          </ul>
          <Pagination page={view.page} pageSize={size} total={sources.length}
            onPage={onPage} noun="sources" />
        </>
      )}
    </section>
  )
}

export function LiveDatasets({ datasets, page, size, onPage }: {
  datasets: DatasetSummary[]
  page: number
  size: number
  onPage: (n: number) => void
}) {
  const view = paginate(datasets, page, size)

  return (
    <section className={CARD} data-testid="datasets-card">
      <div className={HEAD}>
        <h2 className="text-lead font-semibold text-ink">Datasets</h2>
        <span className="text-small text-ink-muted">{datasets.length}</span>
      </div>
      {datasets.length === 0 ? (
        <p className="px-4 py-8 text-center text-small text-ink-muted">
          No datasets yet.
        </p>
      ) : (
        <>
          <ul>
            {view.slice.map((d) => (
              <li key={d.name} className={ROW}>
                <span className="grid min-w-[220px] flex-1 gap-0.5">
                  <span className="font-semibold text-ink">{d.name}</span>
                  <span className="text-small text-ink-secondary">{d.description}</span>
                </span>
                <span className="flex gap-1">
                  <Tag>{d.fields} fields</Tag>
                  {d.measures > 0 && <Tag>summarisable</Tag>}
                  {/* Whether an embedded end customer sees only their own rows
                      is the one property of a dataset somebody should not have
                      to open the file to learn. */}
                  {d.rowScoped && <Tag>row scoped</Tag>}
                </span>
                <span className="text-small text-ink-secondary">{d.sources.join(', ')}</span>
              </li>
            ))}
          </ul>
          <Pagination page={view.page} pageSize={size} total={datasets.length}
            onPage={onPage} noun="datasets" />
        </>
      )}
    </section>
  )
}
