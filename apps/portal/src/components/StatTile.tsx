import { Sparkline } from './Sparkline'
import { percent } from '../lib/format'

interface Props {
  label: string
  value: string
  /** Signed change against a named period. */
  delta?: number
  deltaPeriod?: string
  /** False when a rise is bad — overdue amounts, error rates, days late. */
  upIsGood?: boolean
  trend?: number[]
  /** Exactly one tile per view may be the hero. */
  hero?: boolean
}

/**
 * A single number, not a one-bar chart. Delta colour encodes direction ×
 * whether up is good, and is always paired with an arrow so it never relies
 * on colour alone.
 */
export function StatTile({
  label, value, delta, deltaPeriod, upIsGood = true, trend, hero,
}: Props) {
  const good = delta === undefined || delta === 0 ? null : delta > 0 === upIsGood

  return (
    <div className="rounded-lg border border-line bg-surface p-4 shadow-card">
      <p className="mb-2 text-small text-ink-secondary">{label}</p>
      <div className="flex items-end justify-between gap-3">
        <span className={`font-semibold leading-[1.1] tracking-tight text-ink
                          ${hero ? 'text-hero' : 'text-display'}`}>
          {value}
        </span>
        {trend && <Sparkline values={trend} />}
      </div>
      {delta !== undefined && (
        <p className={`mt-2 flex items-baseline gap-1 text-small
          ${good === null ? 'text-ink-secondary' : good ? 'text-delta-good' : 'text-delta-bad'}`}>
          <span aria-hidden>{delta > 0 ? '↑' : delta < 0 ? '↓' : '→'}</span>
          <span className="tabular-nums">{percent(delta)}</span>
          {deltaPeriod && <span className="text-ink-muted">vs {deltaPeriod}</span>}
        </p>
      )}
    </div>
  )
}
