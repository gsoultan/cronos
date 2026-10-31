import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { RowMenu } from './RowMenu'
import { Tag } from './StatusPill'
import { ApiError, runSchedule } from '../lib/api'
import { EmptyState } from './EmptyState'
import type { ScheduleSummary } from '../lib/api'

const CARD = 'overflow-hidden rounded-lg border border-line bg-surface shadow-card'
/* As on the Data page: the row opens the editor, Run now and ⋯ sit above it. */
const ROW = `relative flex flex-wrap items-center gap-4 border-b border-line px-4 py-3
             last:border-b-0 hover:bg-hover`

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
              <Link to="/schedules/$name/edit" params={{ name: s.name }}
                className="font-semibold text-ink no-underline after:absolute after:inset-0">
                {s.title || s.name}
              </Link>
              <span className="text-small text-ink-secondary">
                {s.description ?? `${s.report} · ${s.output}`}
              </span>
            </span>

            <span className="flex flex-wrap gap-1">
              {s.bursts && <Tag title={`One document per row of ${s.over}`}>One per recipient</Tag>}
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
              })}` : <span title="This server runs no scheduler, so nothing here fires on its own. Run now still works.">
                Scheduler off on this server</span>}
            </span>

            <span className="relative z-10 ml-auto flex shrink-0 items-center gap-2">
              <RunNow name={s.name} />
              <RowMenu kind="Schedule" name={s.name} label={s.title || s.name} />
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
