import { useMemo, useState } from 'react'
import { Button, Select, Textarea } from '@mantine/core'
import { Field } from './form/Field'
import { explainReport, type ReportBlock, type RunFilters } from '../lib/api'
import { explainTargets, suggestions, type ExplainTarget } from '../lib/explain'

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
  about: string
  question: string
  answer?: string
  error?: string
}

/**
 * Asking what a report means.
 *
 * A reader picks the whole report or one block, asks in their own words or
 * picks a suggested question, and gets a short reading of the figures they can
 * see. Every answer says it was generated, because a model's reading of
 * somebody's numbers is a starting point rather than a finding.
 *
 * Earlier answers stay above the box, newest last, so a follow-up can be read
 * against the answer it follows. Each is its own request: the server keeps no
 * conversation, and the panel says nothing that implies it does.
 */
export function ExplainPanel({ reportName, reportLabel, blocks, filters, onClose }: Props) {
  const targets = useMemo(() => explainTargets(blocks), [blocks])
  const [about, setAbout] = useState('report')
  const [question, setQuestion] = useState('')
  const [turns, setTurns] = useState<Turn[]>([])
  const [asking, setAsking] = useState(false)

  const target = targets.find((t) => key(t.block) === about) ?? WHOLE

  async function ask(text: string) {
    const turn: Turn = { about: target.label, question: text.trim() }
    setAsking(true)
    try {
      const got = await explainReport(reportName, {
        filters, block: target.block, question: turn.question || undefined,
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
          <h2 className="text-lead font-semibold text-ink">Explain “{reportLabel}”</h2>
          <p className="mt-1 text-small text-ink-secondary">
            Plain-language answers about the figures you can see, with the filters applied.
          </p>
        </div>
        <Button variant="subtle" color="gray" size="xs" onClick={onClose}>Close</Button>
      </div>

      {turns.length > 0 && (
        <ol className="grid gap-4 border-b border-line p-4" data-testid="explain-answers">
          {turns.map((t, i) => <Answer key={i} turn={t} />)}
        </ol>
      )}

      <div className="grid max-w-[640px] gap-4 p-4">
        <Field label="About">
          <Select data-testid="explain-about" allowDeselect={false} value={about}
            onChange={(v) => setAbout(v ?? 'report')}
            data={targets.map((t) => ({ value: key(t.block), label: t.label }))} />
        </Field>

        <Field label="Your question" required={false}
          help="Leave it empty for a summary of what this shows.">
          <Textarea data-testid="explain-question" autosize minRows={2} maxLength={500}
            value={question} onChange={(e) => setQuestion(e.currentTarget.value)}
            placeholder="For example: why is outstanding higher than last month?"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey && !asking) {
                e.preventDefault()
                void ask(question)
              }
            }} />
        </Field>

        <div className="flex flex-wrap gap-2">
          {suggestions(target).map((s) => (
            <Button key={s} variant="default" size="xs" radius="xl" disabled={asking}
              data-testid="explain-suggestion" onClick={() => void ask(s)}>{s}</Button>
          ))}
        </div>

        <div>
          <Button loading={asking} onClick={() => void ask(question)} data-testid="explain-ask">
            {question.trim() ? 'Ask' : 'Explain'}
          </Button>
        </div>
      </div>
    </section>
  )
}

function Answer({ turn }: { turn: Turn }) {
  return (
    <li className="grid gap-1" data-testid="explain-answer">
      <p className="text-caption text-ink-muted">
        {turn.about}{turn.question ? ` · “${turn.question}”` : ''}
      </p>
      {turn.answer !== undefined ? (
        <>
          {/* Line breaks kept and nothing parsed: the answer is text, and
              rendering a model's output as markup is how a cell value in a
              warehouse becomes a link on somebody's screen. */}
          <p className="whitespace-pre-line text-body text-ink" data-testid="explain-text">
            {turn.answer}
          </p>
          <p className="text-caption text-ink-muted">
            Generated by AI from the figures shown. Check anything important against the report.
          </p>
        </>
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
