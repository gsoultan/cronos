import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Select, Textarea } from '@mantine/core'
import { Field } from './form/Field'
import { explainReport, type ReportBlock, type RunFilters } from '../lib/api'
import { explainTargets, history, mostHistory, suggestions, type ExplainTarget } from '../lib/explain'

interface Props {
  /** The report's API name, which is what the server renders again. */
  reportName: string
  reportLabel: string
  blocks: ReportBlock[]
  /** The filters on screen, so the answer is about the numbers being read. */
  filters: RunFilters
  onClose: () => void
}

interface Turn {
  block?: number
  about: string
  question: string
  answer?: string
  error?: string
}

/**
 * A conversation about a report.
 *
 * A reader picks the whole report or one block, asks in their own words or
 * picks a suggested question, and gets a reading of the figures they can see
 * or ideas for what to do about them. Follow-ups build on what was said: the
 * panel sends the conversation back with each question, since the server keeps
 * none. Every answer says it was generated, because a model's reading of
 * somebody's numbers is a starting point rather than a finding.
 *
 * Changing the filters starts again. The answers so far were about other
 * numbers, and a follow-up read against them would mix the two.
 */
export function ExplainPanel({ reportName, reportLabel, blocks, filters, onClose }: Props) {
  const targets = useMemo(() => explainTargets(blocks), [blocks])
  const [about, setAbout] = useState('report')
  const [question, setQuestion] = useState('')
  const [turns, setTurns] = useState<Turn[]>([])
  const [asking, setAsking] = useState(false)
  const end = useRef<HTMLDivElement>(null)

  const filterKey = JSON.stringify(filters)
  useEffect(() => { setTurns([]) }, [filterKey])
  useEffect(() => { end.current?.scrollIntoView?.({ block: 'nearest' }) }, [turns.length])

  const target = targets.find((t) => key(t.block) === about) ?? WHOLE
  const sofar = history(turns)
  const full = sofar.length >= mostHistory

  async function ask(text: string) {
    if (asking || full) return
    const turn: Turn = { block: target.block, about: target.label, question: text.trim() }
    setAsking(true)
    try {
      const got = await explainReport(reportName, {
        filters, block: target.block, question: turn.question || undefined,
        history: sofar.length ? sofar : undefined,
      })
      turn.answer = got.text
      setQuestion('')
    } catch (err) {
      turn.error = err instanceof Error ? err.message : 'No explanation is available right now.'
    } finally {
      setTurns((all) => [...all, turn])
      setAsking(false)
    }
  }

  return (
    <section className="rounded-lg border border-line bg-surface shadow-card"
      data-testid="explain-panel">
      <div className="flex items-start justify-between gap-4 border-b border-line p-4">
        <div>
          <h2 className="text-lead font-semibold text-ink">Ask about “{reportLabel}”</h2>
          <p className="mt-1 text-small text-ink-secondary">
            Explanations and ideas about the figures you can see, with the filters applied.
            Ask follow-up questions in the same conversation.
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          {turns.length > 0 && (
            <Button variant="default" size="xs" disabled={asking} data-testid="explain-reset"
              onClick={() => setTurns([])}>New conversation</Button>
          )}
          <Button variant="subtle" color="gray" size="xs" onClick={onClose}>Close</Button>
        </div>
      </div>

      {turns.length > 0 && (
        <ol className="grid max-h-[60vh] gap-5 overflow-y-auto border-b border-line p-4"
          data-testid="explain-answers" aria-live="polite">
          {turns.map((t, i) => <Exchange key={i} turn={t} />)}
          <div ref={end} />
        </ol>
      )}

      <div className="grid max-w-[640px] gap-4 p-4">
        <Field label="About">
          <Select data-testid="explain-about" allowDeselect={false} value={about}
            onChange={(v) => setAbout(v ?? 'report')}
            data={targets.map((t) => ({ value: key(t.block), label: t.label }))} />
        </Field>

        <Field label={turns.length ? 'Follow-up question' : 'Your question'} required={false}
          help="Leave it empty for an explanation of what this shows.">
          <Textarea data-testid="explain-question" autosize minRows={2} maxLength={500}
            value={question} disabled={full} onChange={(e) => setQuestion(e.currentTarget.value)}
            placeholder="For example: how could we bring outstanding down?"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                void ask(question)
              }
            }} />
        </Field>

        {full ? (
          <p className="text-small text-ink-secondary" data-testid="explain-full">
            This conversation is as long as it can get. Start a new one to keep asking.
          </p>
        ) : (
          <div className="flex flex-wrap gap-2">
            {suggestions(target, turns.length > 0).map((s) => (
              <Button key={s} variant="default" size="xs" radius="xl" disabled={asking}
                data-testid="explain-suggestion" onClick={() => void ask(s)}>{s}</Button>
            ))}
          </div>
        )}

        <div>
          <Button loading={asking} disabled={full} onClick={() => void ask(question)}
            data-testid="explain-ask">
            {question.trim() ? 'Ask' : 'Explain'}
          </Button>
        </div>
      </div>
    </section>
  )
}

function Exchange({ turn }: { turn: Turn }) {
  return (
    <li className="grid gap-2" data-testid="explain-answer">
      <div className="max-w-[80%] justify-self-end rounded-lg border border-line bg-surface-sunken px-3 py-2 text-small text-ink"
        data-testid="explain-asked">
        <p className="text-caption text-ink-muted">{turn.about}</p>
        <p>{turn.question || 'Explain what this shows.'}</p>
      </div>
      {turn.answer !== undefined ? (
        <div className="grid gap-1">
          {/* Line breaks kept and nothing parsed: the answer is text, and
              rendering a model's output as markup is how a cell value in a
              warehouse becomes a link on somebody's screen. */}
          <p className="whitespace-pre-line text-body text-ink" data-testid="explain-text">
            {turn.answer}
          </p>
          <p className="text-caption text-ink-muted">
            Generated by AI from the figures shown. Treat advice as ideas to test, and check
            anything important against the report.
          </p>
        </div>
      ) : (
        <p role="alert" data-testid="explain-error"
          className="rounded-md border border-serious/30 bg-serious/10 px-3 py-2 text-small text-ink">
          {turn.error}
        </p>
      )}
    </li>
  )
}

const WHOLE: ExplainTarget = { label: 'The whole report' }

function key(block?: number): string {
  return block === undefined ? 'report' : String(block)
}
