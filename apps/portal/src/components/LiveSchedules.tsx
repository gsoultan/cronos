import { Link } from '@tanstack/react-router'
import { Tag } from './StatusPill'
import { EmptyState } from './EmptyState'
import type { ScheduleSummary } from '../lib/api'

const CARD = 'overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const ROW = 'flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0'

/**
 * Schedules as the server has them.
 *
 * `next` is the useful column and the one a fixture cannot have: when this
 * actually fires, computed from the cron expression in its own timezone by the
 * loop that will honour it.
 */
export function LiveSchedules({ schedules }: { schedules: ScheduleSummary[] }) {
  if (schedules.length === 0) {
    return (
      <EmptyState title="Nothing is scheduled"
        description="A schedule turns a report into something that arrives without being asked for." />
    )
  }

  return (
    <section className={CARD} data-testid="schedules-card">
      <ul>
        {schedules.map((s) => (
          <li key={s.name} className={ROW}>
            <span className="grid min-w-[220px] flex-1 gap-0.5">
              <span className="font-semibold text-ink">{s.title || s.name}</span>
              <span className="text-small text-ink-secondary">
                {s.description ?? `${s.report} · ${s.output}`}
              </span>
            </span>

            <span className="flex flex-wrap gap-1">
              {s.bursts && <Tag>one per {s.over}</Tag>}
              {s.channels.map((c) => <Tag key={c}>{c}</Tag>)}
            </span>

            <span className="text-small text-ink-secondary" title={`${s.cron} ${s.timezone}`}>
              {s.timezone}
            </span>

            <span data-testid="next-run" className="text-caption text-ink-muted">
              {/* Nothing rather than a time nothing will honour: a schedule
                  shown as "next: 06:00" by a server with no scheduler armed is
                  a promise the deployment has not made. */}
              {s.next ? `next ${new Date(s.next).toLocaleString('en-GB', {
                dateStyle: 'medium', timeStyle: 'short',
              })}` : 'not scheduled here'}
            </span>

            <Link to="/schedules/$name/edit" params={{ name: s.name }}
              className="ml-auto shrink-0 text-small text-ink-muted underline hover:text-ink">
              Edit
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}
