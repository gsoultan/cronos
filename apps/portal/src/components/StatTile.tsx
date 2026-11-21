import { Sparkline } from './Sparkline'
import { percent } from '../lib/format'
import type { Goal } from '@cronos/charts'

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
  /** The figure against its target, as the engine worked it out. */
  goal?: Goal
}

/**
 * A single number, not a one-bar chart. Delta colour encodes direction ×
 * whether up is good, and is always paired with an arrow so it never relies
 * on colour alone.
 */
export function StatTile({
  label, value, delta, deltaPeriod, upIsGood = true, trend, hero, goal,
}: Props) {
  const good = delta === undefined || delta === 0 ? null : delta > 0 === upIsGood

  return (
    <div className="rounded-lg border border-line bg-surface p-4 shadow-card">
      <p className="mb-2 text-small text-ink-secondary">{label}</p>
      <div className="flex items-end justify-between gap-3">
        {/* min-w-0 so the number cannot push its own tile open. A flex child
            will not shrink below its content by default, and a hero figure is
            wide enough to reach past the card it lives in. */}
        <span className={`min-w-0 font-semibold leading-[1.1] tracking-tight text-ink
                          ${size(value, hero)}`}>
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
      {goal && <GoalBar goal={goal} />}
    </div>
  )
}

/**
 * How far along the figure is: a bar to its target, full and in the good
 * tone once it is reached, with a tick where the target fell when the figure
 * is past it — so 120% is not drawn as the same bar as 100%.
 */
export function GoalBar({ goal }: { goal: Goal }) {
  const done = goal.share >= 1
  const width = Math.max(0, Math.min(goal.share, 1)) * 100
  return (
    <div className="mt-3" data-testid="stat-goal">
      <div className="relative h-1.5 rounded-full bg-line" role="img"
        aria-label={`${goal.said} of ${goal.label.toLowerCase()} ${goal.value}`}>
        <div className={`absolute inset-y-0 left-0 rounded-full ${done ? 'bg-delta-good' : 'bg-accent'}`}
          style={{ width: `${width}%` }} />
        {goal.share > 1 && (
          <div className="absolute -inset-y-1 w-0.5 -translate-x-1/2 bg-ink"
            style={{ left: `${100 / goal.share}%` }} />
        )}
      </div>
      <p className="mt-1.5 text-caption text-ink-muted tabular-nums">
        <span className="font-semibold text-ink">{goal.said}</span>
        {' '}of {goal.label.toLowerCase()} {goal.value}
      </p>
    </div>
  )
}

/**
 * How big a figure can be without leaving its tile.
 *
 * The hero size is 48px, and the tile it sits in is a quarter of the report
 * width. "154,651.50" at 48px is wider than that — so the primary number of the
 * demo's primary report rendered *through* the tile beside it, half of it
 * behind the neighbouring card. It looked like a rendering fault rather than a
 * number, which on the one figure a reader looks at first is as bad as being
 * wrong.
 *
 * It went unseen because the fixture's hero value is a short one; a real
 * warehouse produced a longer one on the first report anybody built.
 *
 * Steps rather than a fluid clamp: a currency figure is read by comparing it
 * with the one beside it, and two tiles whose type size drifts continuously
 * with their contents are harder to compare than two at the same size. Thirteen
 * characters is roughly where 48px stops fitting a quarter-width tile — a
 * billion with decimals, or a long non-currency string.
 */
export function size(value: string, hero?: boolean): string {
  if (!hero) return 'text-display'
  if (value.length <= 9) return 'text-hero'
  if (value.length <= 13) return 'text-[36px]'
  // Past this, the tile is the constraint rather than the type scale, and a
  // figure nobody can read is worse than a small one.
  return 'text-display'
}
