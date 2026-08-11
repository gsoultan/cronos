import { useState } from 'react'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { Tag } from '../components/StatusPill'
import { relativeTime } from '../lib/format'
import { useRun, useRuns } from '../lib/useRuns'
import type { Run, RunDelivery } from '../lib/api'

const CARD = 'mb-6 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const ROW = 'flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0'

/**
 * What ran, and who received it.
 *
 * The question a scheduler creates. Everything else in the portal is something
 * somebody is looking at while it happens; a schedule fires at 06:00 on the
 * first of the month into an empty room, and the only evidence it worked is
 * here.
 *
 * Server only. A fixture of plausible deliveries would be a page that looks
 * like an audit and answers nothing, which is worse than a page that says it
 * has nothing to show.
 */
export function ActivityPage() {
  const { data, isPending, error, live } = useRuns()
  const [open, setOpen] = useState<string | null>(null)

  if (!live) {
    return (
      <>
        <PageHeader title="Activity" description="What ran, and who received it." />
        <EmptyState title="Nothing has run here"
          description="Run history comes from a server. This portal is showing sample data, which has none — connect it to a cronos deployment and every send appears here." />
      </>
    )
  }
  if (isPending) {
    return <p data-testid="runs-loading" className="p-8 text-center text-ink-muted">Loading…</p>
  }
  if (error) {
    return (
      <EmptyState title="Could not read the run history"
        description={error instanceof Error ? error.message : 'Unknown error.'} />
    )
  }

  const runs = data?.runs ?? []

  return (
    <>
      <PageHeader title="Activity"
        description="Every scheduled send, newest first. A partial result is its own state: the successes went out, and only the failures need attention." />

      {runs.length === 0 ? (
        <EmptyState title="Nothing has run yet"
          description="Schedules record every send here — which report, which version of it, and what reached each recipient." />
      ) : (
        <section className={CARD} data-testid="runs-card">
          <ul>
            {runs.map((r) => (
              <li key={r.id}>
                <div className={ROW}>
                  <span className="grid min-w-[220px] flex-1 gap-0.5">
                    <span className="font-semibold text-ink">{r.schedule || r.report}</span>
                    <span className="text-small text-ink-secondary">
                      {r.report}
                      {r.output ? ` · ${r.output}` : ''}
                      {r.periodEnd ? ` · period to ${r.periodEnd}` : ''}
                    </span>
                  </span>

                  <span className="text-small text-ink-secondary" data-testid="run-count">
                    {r.delivered.toLocaleString('en')} of {r.recipients.toLocaleString('en')}
                  </span>

                  <Status run={r} />

                  {/* The version, because a run is only reproducible against
                      the exact bytes that produced it — and this is the one
                      place that number is worth showing. */}
                  {r.reportVersion && (
                    <span className="font-mono text-caption text-ink-muted" title="the definition this ran">
                      {r.reportVersion.replace('sha256:', '')}
                    </span>
                  )}

                  <span className="text-caption text-ink-muted">{relativeTime(r.startedAt)}</span>

                  <button type="button" data-testid="run-toggle"
                    onClick={() => setOpen(open === r.id ? null : r.id)}
                    className="ml-auto shrink-0 cursor-pointer text-small text-ink-muted underline hover:text-ink">
                    {open === r.id ? 'Hide' : 'Recipients'}
                  </button>
                </div>

                {open === r.id && <Deliveries id={r.id} />}
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  )
}

/**
 * A partial send is not a failure and not a success.
 *
 * The successes went out and cannot be unsent; only the remainder needs doing.
 * Colouring it as failed would send somebody to re-run all 812.
 */
function Status({ run }: { run: Run }) {
  const label = run.status === 'delivered' ? 'Delivered'
    : run.status === 'running' ? 'Sending'
      : run.status === 'partial' ? `${run.recipients - run.delivered} failed`
        : 'Failed'

  const tone = run.status === 'delivered' ? 'bg-good/15 text-delta-good'
    : run.status === 'running' ? 'bg-sunken text-ink-secondary'
      : 'bg-serious/20 text-ink'

  return (
    <span data-testid="run-status" className={`rounded-full px-2 py-px text-micro font-medium ${tone}`}>
      {label}
    </span>
  )
}

/** Every recipient the run attempted, and what happened to each. */
function Deliveries({ id }: { id: string }) {
  const { data, isPending, error } = useRun(id)

  if (isPending) {
    return <p className="px-4 py-6 text-center text-small text-ink-muted">Loading recipients…</p>
  }
  if (error || !data) {
    return (
      <p className="px-4 py-6 text-center text-small text-ink-muted">
        {error instanceof Error ? error.message : 'Could not read this run.'}
      </p>
    )
  }
  if (data.deliveries.length === 0) {
    return (
      <p className="px-4 py-6 text-center text-small text-ink-muted">
        This run recorded no deliveries.
      </p>
    )
  }

  /* Failures first. On a partial send of 812 the two that failed are the whole
     reason somebody opened this, and making them scroll for them is the same
     as not having the list. */
  const ordered = data.deliveries.toSorted((a, b) => rank(a) - rank(b))

  return (
    <ul data-testid="run-deliveries" className="border-b border-line bg-sunken">
      {ordered.map((d, i) => (
        <li key={`${d.recipient}-${i}`}
          className="flex flex-wrap items-center gap-4 border-b border-line/60 px-4 py-2 last:border-b-0">
          <span className="min-w-[180px] flex-1 text-small text-ink">{d.recipient}</span>
          {/* Where it actually went. For email the destination is the address;
              for a file channel it is the recipient again and the filename is
              the answer, so showing the destination alone would print the
              recipient twice and say nothing. */}
          <span className="text-small text-ink-secondary" data-testid="delivery-destination">
            {d.filename || (d.destination === d.recipient ? '' : d.destination)}
          </span>
          <Tag>{d.channel}</Tag>
          {d.attempts > 1 && (
            <span className="text-caption text-ink-muted">{d.attempts} attempts</span>
          )}
          <span className={`rounded-full px-2 py-px text-micro font-medium ${
            failed(d) ? 'bg-serious/20 text-ink' : 'bg-good/15 text-delta-good'}`}>
            {d.status}
          </span>
          {/* The channel's own sentence. It is the only party that knows
              whether this was a bounced address or a refused attachment. */}
          {d.error && (
            <span className="w-full text-caption text-ink-secondary">{d.error}</span>
          )}
        </li>
      ))}
    </ul>
  )
}

function failed(d: RunDelivery): boolean {
  return d.status !== 'delivered' && d.status !== 'sent'
}

function rank(d: RunDelivery): number {
  return failed(d) ? 0 : 1
}
