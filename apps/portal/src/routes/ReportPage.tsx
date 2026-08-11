import { useMemo, useState } from 'react'
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
import { applyFilter } from '../lib/applyFilter'

const EMPTY: Group = { id: 'root', kind: 'group', join: 'and', children: [] }

export function ReportPage() {
  const { name } = useParams({ from: '/reports/$name' })
  const report = reports.find((r) => r.name === name)
  const dataset = datasets.find((d) => d.name === report?.dataset)
  const [filter, setFilter] = useState<Group>(EMPTY)
  const [sharing, setSharing] = useState(false)

  const rows = useMemo(
    () => (dataset ? applyFilter(invoiceRows, filter, dataset.fields) : []),
    [filter, dataset],
  )

  if (!report || !dataset) {
    return (
      <EmptyState title="That report does not exist"
        description="It may have been renamed or deleted."
        action={<Button component={Link} to="/">Back to reports</Button>} />
    )
  }

  const billed = rows.reduce((s, r) => s + r.total, 0)
  const overdue = rows.filter((r) => r.status === 'Overdue')
  const outstanding = overdue.reduce((s, r) => s + r.total, 0)

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

      {rows.length === 0 ? (
        <EmptyState
          title="No rows match these filters"
          description="Try removing a condition — the filter summary above shows what is being applied."
          action={<Button variant="default" onClick={() => setFilter(EMPTY)}>Clear all filters</Button>}
        />
      ) : (
        <>
          {/* The hero carries no sparkline: a 12-point line beside a 48px
              figure reads as debris, and the number is the headline. */}
          <div className="mb-6 grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(200px,1fr))]">
            <StatTile hero label="Total billed" value={currency(billed)}
              delta={6.4} deltaPeriod="last month" />
            <StatTile label="Outstanding" value={currency(outstanding)}
              delta={12.1} deltaPeriod="last month" upIsGood={false}
              trend={outstandingTrend} />
            <StatTile label="Overdue invoices" value={overdue.length.toLocaleString('en')}
              delta={-3.2} deltaPeriod="last month" upIsGood={false}
              trend={overdueCountTrend} />
            <StatTile label="Collection rate" value="95.6%"
              delta={2.5} deltaPeriod="last month"
              trend={collectionsTrend.map((c) => c.value)} />
          </div>

          <div className="mb-6 grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(400px,1fr))]">
            <ColumnChart title="Billed by month" subtitle="Split by invoice status"
              data={billedByMonth} series={STATUSES} />
            <LineChart title="Collection rate" subtitle="Share of invoices paid within terms"
              data={collectionsTrend} target={{ value: 93, label: 'Target' }} />
          </div>

          <Panel title="Invoices" meta={`${rows.length.toLocaleString('en')} rows`} flush>
            <DataTable fields={dataset.fields} rows={rows}
              totalLabel={`${rows.length.toLocaleString('en')} rows · ${currency(billed)} total`} />
          </Panel>
        </>
      )}
    </>
  )
}
