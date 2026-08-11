import type { ReactNode } from 'react'
import { Button } from '@mantine/core'

export interface Step {
  id: string
  label: string
  /** One line: what this step is for. */
  hint: string
}

interface Props {
  steps: Step[]
  current: number
  /** A step is reachable once every step before it is complete. */
  completed: number
  onStep: (index: number) => void
  onBack: () => void
  onNext: () => void
  canAdvance: boolean
  nextLabel?: string
  busy?: boolean
  children: ReactNode
}

/**
 * A wizard, not one long form, wherever a step's answers change what the next
 * step should even ask — connecting Postgres and connecting a REST API have
 * almost no fields in common, and showing both at once is how a form becomes
 * frightening.
 *
 * Completed steps are clickable so going back to check something is not a
 * gamble; steps ahead are not, because their questions may not exist yet.
 */
export function Wizard({
  steps, current, completed, onStep, onBack, onNext,
  canAdvance, nextLabel, busy, children,
}: Props) {
  const last = current === steps.length - 1

  return (
    <div className="grid items-start gap-6 md:grid-cols-[260px_1fr]">
      <ol className="grid gap-1">
        {steps.map((s, i) => {
          const done = i < completed
          const isCurrent = i === current
          return (
            <li key={s.id}>
              <button type="button" disabled={i > completed}
                aria-current={isCurrent ? 'step' : undefined}
                onClick={() => onStep(i)}
                className={`flex w-full items-start gap-3 rounded-md p-3 text-left text-ink
                  ${i > completed ? 'cursor-default opacity-50' : 'cursor-pointer hover:bg-hover'}
                  ${isCurrent ? 'bg-accent-wash' : ''}`}>
                <span aria-hidden
                  className={`grid size-[22px] shrink-0 place-items-center rounded-full
                    text-caption font-semibold ${
                      done ? 'bg-good text-white'
                        : isCurrent ? 'bg-accent text-white'
                        : 'bg-sunken text-ink-secondary'}`}>
                  {done ? '✓' : i + 1}
                </span>
                <span>
                  <span className="block text-small font-semibold">{s.label}</span>
                  <span className="block text-caption text-ink-muted">{s.hint}</span>
                </span>
              </button>
            </li>
          )
        })}
      </ol>

      <div className="min-w-0">
        {children}
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="default" onClick={onBack} disabled={current === 0 || busy}>Back</Button>
          <Button onClick={onNext} disabled={!canAdvance} loading={busy}>
            {nextLabel ?? (last ? 'Finish' : 'Continue')}
          </Button>
        </div>
      </div>
    </div>
  )
}
