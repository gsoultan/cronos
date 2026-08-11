import { useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { Button, Menu } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { StatTile } from '../components/StatTile'
import { ColumnChart } from '../components/ColumnChart'
import { LineChart } from '../components/LineChart'
import { DataTable } from '../components/DataTable'
import { Panel } from '../components/Panel'
import { FilterPanel } from '../components/FilterPanel'
import { EmptyState } from '../components/EmptyState'
import { SharePanel } from '../components/share/SharePanel'
import type { Group } from '../lib/types'
import {
  billedByMonth, collectionsTrend, datasets, invoiceRows, outstandingTrend,
  overdueCountTrend, reports, STATUSES,
} from '../lib/mock'
import { currency } from '../lib/format'
import { useRowWorker } from '../lib/useRowWorker'
import { useReport } from '../lib/useReport'
import { LiveReport } from '../components/LiveReport'

const EMPTY: Group = { id: 'root', kind: 'group', join: 'and', children: [] }

/**
 * A report, from wherever the numbers actually come from.
 *
 * A dispatcher and nothing else. The two branches have entirely different
 * hooks — one runs a filter worker over four thousand fixture rows, the other
 * runs a query — and putting both in one component would mount the worker for
 * a page that never shows it. React would also have to keep the hook order
 * stable across a branch that changes what a report *is*.
 */
export function ReportPage() {
  const { name } = useParams({ from: '/reports/$name' })
  const live = useReport(name)

  if (live.live) return <ServerReport name={name} query={live} />
  return <SampleReport name={name} />
}

function SampleReport({ name }: { name: string }) {
  const report = reports.find((r) => r.name === name)
  const dataset = datasets.find((d) => d.name === report?.dataset)
  const [filter, setFilter] = useState<Group>(EMPTY)
  const [sharing, setSharing] = useState(false)

  /* Filtering four thousand rows on every keystroke belongs off the main
     thread. The worker keeps the rows and returns totals plus a page. */
  const result = useRowWorker({
    rows: invoiceRows as unknown as Record<string, unknown>[],
    fields: dataset?.fields ?? [],
    filter,
    sum: ['total'],
    count: [{ field: 'status', equals: 'Overdue' }],
  })

  if (!report || !dataset) {
    return (
      <EmptyState title="That report does not exist"
        description="It may have been renamed or deleted."
        action={<Button component={Link} to="/">Back to reports</Button>} />
    )
  }

  /* Aggregates come from the whole filtered set, not the page on screen. */
  const billed = result.sums.total ?? 0
  const overdueCount = result.counts['status=Overdue'] ?? 0
  const outstanding = billed * 0.11

  return (
    <>
      <PageHeader
        eyebrow={report.folder}
        title={report.label}
        description={report.description}
        actions={
          <>
            <Menu shadow="md" position="bottom-end">
              <Menu.Target><Button variant="default">Export</Button></Menu.Target>
              <Menu.Dropdown>
                <Menu.Item>PDF — print-ready statement</Menu.Item>
                <Menu.Item>Excel — one row per invoice</Menu.Item>
                <Menu.Item>CSV — raw data</Menu.Item>
              </Menu.Dropdown>
            </Menu>
            <Button variant="default" onClick={() => setSharing((v) => !v)}
              data-testid="share-button">Share</Button>
            <Button>Schedule</Button>
          </>
        }
      />

      {sharing && (
        <div className="mb-6">
          <SharePanel reportLabel={report.label} projectName={report.folder}
            outputs={report.outputs} onClose={() => setSharing(false)} />
        </div>
      )}

      <FilterPanel fields={dataset.fields} value={filter} onChange={setFilter} onApply={() => {}} />

      {result.total === 0 ? (
        <EmptyState
          title="No rows match these filters"
          description="Try removing a condition — the filter summary above shows what is being applied."
          action={<Button variant="default" onClick={() => setFilter(EMPTY)}>Clear all filters</Button>}
        />
      ) : (
        <>
          {/* The hero carries no sparkline: a 12-point line beside a 48px
              figure reads as debris, and the number is the headline. */}
          <div className="mb-6 grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(min(200px,100%),1fr))]">
            <StatTile hero label="Total billed" value={currency(billed)}
              delta={6.4} deltaPeriod="last month" />
            <StatTile label="Outstanding" value={currency(outstanding)}
              delta={12.1} deltaPeriod="last month" upIsGood={false}
              trend={outstandingTrend} />
            <StatTile label="Overdue invoices" value={overdueCount.toLocaleString('en')}
              delta={-3.2} deltaPeriod="last month" upIsGood={false}
              trend={overdueCountTrend} />
            <StatTile label="Collection rate" value="95.6%"
              delta={2.5} deltaPeriod="last month"
              trend={collectionsTrend.map((c) => c.value)} />
          </div>

          <div className="mb-6 grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(min(400px,100%),1fr))]">
            <ColumnChart title="Billed by month" subtitle="Split by invoice status"
              data={billedByMonth} series={STATUSES} />
            <LineChart title="Collection rate" subtitle="Share of invoices paid within terms"
              data={collectionsTrend} target={{ value: 93, label: 'Target' }} />
          </div>

          <Panel title="Invoices" flush
            meta={<span data-testid="row-meta">
              {result.total.toLocaleString('en')} rows
              {result.offloaded && (
                <span className="ml-2 text-ink-muted">
                  filtered in {result.ms < 1 ? '<1' : result.ms}ms, off the main thread
                </span>
              )}
            </span>}>
            <DataTable fields={dataset.fields} rows={result.rows}
              totalLabel={`${result.total.toLocaleString('en')} rows · ${currency(billed)} total`} />
          </Panel>
        </>
      )}
    </>
  )
}

/** A report as the server computed it, with the states a request can be in. */
function ServerReport({ name, query }: { name: string; query: ReturnType<typeof useReport> }) {
  if (query.isPending) {
    return <p data-testid="report-loading" className="p-8 text-center text-ink-muted">Loading…</p>
  }
  if (query.error) {
    /* The server's own sentence. It is the only party that knows whether this
       was an expired token or a report that does not exist. */
    return (
      <EmptyState title="This report could not be run"
        description={query.error instanceof Error ? query.error.message : 'Unknown error.'}
        action={<Button component={Link} to="/">Back to reports</Button>} />
    )
  }
  if (!query.data) return null

  return (
    <>
      <PageHeader eyebrow={name} title={query.data.title}
        description={query.data.description} />
      <LiveReport view={query.data} />
    </>
  )
}
