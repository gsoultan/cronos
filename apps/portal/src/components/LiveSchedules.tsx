import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { Tag } from './StatusPill'
import { ApiError, runSchedule } from '../lib/api'
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

            <span className="ml-auto flex shrink-0 items-center gap-3">
              <RunNow name={s.name} />
              <Link to="/schedules/$name/edit" params={{ name: s.name }}
                className="text-small text-ink-muted underline hover:text-ink">
                Edit
              </Link>
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}

/**
 * Runs one schedule now.
 *
 * A monthly schedule is otherwise untestable until the first of the month, and
 * the recipients, the render and the delivery are exactly the parts most
 * likely to be wrong. Discovering it at 06:00 on the 1st is discovering it in
 * front of the customer.
 *
 * Confirmed first, because this sends real documents to real people. The
 * confirmation names the count rather than asking "are you sure" — "are you
 * sure" is a question nobody can answer, and "send 812 documents now" is.
 */
function RunNow({ name }: { name: string }) {
  const [state, setState] = useState<'idle' | 'confirm' | 'running' | 'failed'>('idle')
  const [message, setMessage] = useState('')
  const queries = useQueryClient()
  const navigate = useNavigate()

  async function go() {
    setState('running')
    try {
      await runSchedule(name)
      // Straight to the history, because that is where the answer is. A toast
      // saying "started" would leave somebody on a page that cannot tell them
      // whether it worked.
      await queries.invalidateQueries({ queryKey: ['runs'] })
      await navigate({ to: '/activity' })
    } catch (err) {
      setState('failed')
      setMessage(err instanceof ApiError ? err.message : 'Could not reach the server.')
    }
  }

  if (state === 'failed') {
    return <span className="text-small text-ink" title={message}>Did not run</span>
  }
  if (state === 'running') {
    return <span className="text-small text-ink-muted">Running…</span>
  }
  if (state === 'confirm') {
    return (
      <span className="flex items-center gap-2 text-small">
        <button type="button" onClick={go} data-testid="run-confirm"
          className="cursor-pointer font-medium text-ink underline">Send now</button>
        <button type="button" onClick={() => setState('idle')}
          className="cursor-pointer text-ink-muted underline">Cancel</button>
      </span>
    )
  }
  return (
    <button type="button" onClick={() => setState('confirm')} data-testid="run-now"
      className="cursor-pointer text-small text-ink-muted underline hover:text-ink">
      Run now
    </button>
  )
}
