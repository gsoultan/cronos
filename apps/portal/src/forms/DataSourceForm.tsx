import { useState } from 'react'
import { useForm, useStore } from '@tanstack/react-form'
import { NumberInput, PasswordInput, TextInput, Textarea } from '@mantine/core'
import { Field, fieldError } from '../components/form/Field'
import { IdentifierField } from '../components/form/IdentifierField'
import { FormSection, Callout } from '../components/form/FormShell'
import { Wizard, type Step } from '../components/form/Wizard'
import { all, port as portRule, required, slug, toSlug, url } from '../lib/validators'
import { SOURCE_KINDS, type SourceKind } from '../lib/sources'

const STEPS: Step[] = [
  { id: 'kind', label: 'Choose a source', hint: 'Where the data lives' },
  { id: 'connect', label: 'Connect', hint: 'Address and credentials' },
  { id: 'test', label: 'Test', hint: 'Check it works before saving' },
  { id: 'name', label: 'Name it', hint: 'How your team will find it' },
]

type TestState = { status: 'idle' | 'running' | 'ok' | 'failed'; message?: string; tables?: number }

interface Props {
  onDone: () => void
  onCancel: () => void
}

/**
 * Connecting a source, as a wizard.
 *
 * Postgres and a REST endpoint have almost no fields in common, so one long
 * form would show everyone every field that could ever apply. Choosing the kind
 * first means step two only ever asks what that kind needs.
 *
 * The test step exists because a bad connection discovered later surfaces as a
 * broken report at 6am on the first of the month, with no clue that a password
 * was the cause.
 */
export function DataSourceForm({ onDone, onCancel }: Props) {
  /* Once someone edits the API name, typing in Name must stop
     overwriting it — silently discarding a deliberate edit. */
  const [slugEdited, setSlugEdited] = useState(false)
  const [step, setStep] = useState(0)
  const [completed, setCompleted] = useState(0)
  const [kind, setKind] = useState<SourceKind | null>(null)
  const [test, setTest] = useState<TestState>({ status: 'idle' })

  const form = useForm({
    defaultValues: {
      host: '', port: 5432, database: '', user: '', password: '',
      uri: '', endpoint: '', authHeader: '', filePath: '',
      name: '', slug: '',
    },
    onSubmit: async () => { onDone() },
  })

  const values = useStore(form.store, (s) => s.values)
  const spec = kind ? SOURCE_KINDS.find((k) => k.id === kind)! : null

  function advance() {
    if (step === STEPS.length - 1) return void form.handleSubmit()
    const next = step + 1
    setStep(next)
    setCompleted((c) => Math.max(c, next))
  }

  async function runTest() {
    setTest({ status: 'running' })
    await new Promise((r) => setTimeout(r, 900))
    // Stands in for a real probe until the engine exists.
    setTest({ status: 'ok', message: 'Connected', tables: 24 })
    setCompleted((c) => Math.max(c, 3))
  }

  const canAdvance = (() => {
    switch (step) {
      case 0: return kind !== null
      case 1: return connectionComplete(spec, values)
      case 2: return test.status === 'ok'
      case 3: return values.name.trim().length > 1
      default: return false
    }
  })()

  return (
    <form onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); advance() }}>
      <Wizard
        steps={STEPS} current={step} completed={completed}
        onStep={setStep} onBack={() => setStep((s) => s - 1)} onNext={advance}
        canAdvance={canAdvance} busy={test.status === 'running'}
        nextLabel={step === 2 ? 'Continue' : step === 3 ? 'Save source' : 'Continue'}
      >
        {step === 0 && (
          <FormSection title="What are you connecting?"
            description="cronos reads from each of these in place — nothing is copied or uploaded.">
            <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(210px,1fr))]">
              {SOURCE_KINDS.map((k) => (
                <button key={k.id} type="button"
                  onClick={() => { setKind(k.id); setCompleted((c) => Math.max(c, 1)) }}
                  aria-pressed={kind === k.id}
                  className={`grid cursor-pointer gap-1 rounded-lg border bg-surface p-4 text-left text-ink transition-colors duration-150 ease-out-quick hover:border-accent ${kind === k.id ? 'border-accent bg-accent-wash' : 'border-line'}`}>
                  <span className="text-title leading-none" aria-hidden>{k.icon}</span>
                  <span className="font-semibold">{k.label}</span>
                  <span className="text-small text-ink-secondary">{k.hint}</span>
                  <span className={`mt-1 text-micro font-medium tracking-[0.04em] uppercase
                    ${pushdownTone(k.pushdown)}`}>
                    {k.pushdownLabel}
                  </span>
                </button>
              ))}
            </div>
          </FormSection>
        )}

        {step === 1 && spec && (
          <FormSection title={`Connect to ${spec.label}`} description={spec.connectHint}>
            {spec.shape === 'sql' && (
              <>
                <form.Field name="host" validators={{ onBlur: ({ value }) => required('A host')(value) }}>
                  {(f) => (
                    <Field label="Host" error={fieldError(f.state.meta)}
                      help="The machine cronos should connect to.">
                      <TextInput value={f.state.value} onBlur={f.handleBlur}
                        placeholder="db.internal.acme.com"
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>

                <form.Field name="port" validators={{ onBlur: ({ value }) => portRule(value) }}>
                  {(f) => (
                    <Field label="Port" error={fieldError(f.state.meta)}>
                      <NumberInput value={f.state.value} onBlur={f.handleBlur} w={160}
                        onChange={(v) => f.handleChange(Number(v) || 0)} />
                    </Field>
                  )}
                </form.Field>

                <form.Field name="database" validators={{ onBlur: ({ value }) => required('A database')(value) }}>
                  {(f) => (
                    <Field label="Database" error={fieldError(f.state.meta)}>
                      <TextInput value={f.state.value} onBlur={f.handleBlur}
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>

                <form.Field name="user" validators={{ onBlur: ({ value }) => required('A username')(value) }}>
                  {(f) => (
                    <Field label="Username" error={fieldError(f.state.meta)}
                      help="Give it read-only access. cronos never writes to your data.">
                      <TextInput value={f.state.value} onBlur={f.handleBlur}
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>

                <form.Field name="password">
                  {(f) => (
                    <Field label="Password"
                      help="Stored in your secret backend, never in a report definition.">
                      <PasswordInput value={f.state.value} onBlur={f.handleBlur}
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>
              </>
            )}

            {spec.shape === 'object' && (
              <form.Field name="uri" validators={{ onBlur: ({ value }) => required('A location')(value) }}>
                {(f) => (
                  <Field label="Location" error={fieldError(f.state.meta)}
                    help="A bucket and prefix. Files underneath are read as one table.">
                    <TextInput value={f.state.value} onBlur={f.handleBlur}
                      placeholder="s3://acme-lake/events/"
                      onChange={(e) => f.handleChange(e.currentTarget.value)} />
                  </Field>
                )}
              </form.Field>
            )}

            {spec.shape === 'api' && (
              <>
                <form.Field name="endpoint"
                  validators={{ onBlur: ({ value }) => all(required('An address'), url)(value) }}>
                  {(f) => (
                    <Field label="Address" error={fieldError(f.state.meta)}
                      help="cronos calls this and turns the JSON it returns into a table.">
                      <TextInput value={f.state.value} onBlur={f.handleBlur}
                        placeholder="https://api.example.com/orders"
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>
                <form.Field name="authHeader">
                  {(f) => (
                    <Field label="Authorisation header" required={false}
                      help="Sent with every request. Leave blank for a public endpoint.">
                      <Textarea autosize minRows={2} value={f.state.value} onBlur={f.handleBlur}
                        placeholder="Bearer ${secret:orders_api_token}"
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>
              </>
            )}

            {spec.shape === 'file' && (
              <form.Field name="filePath" validators={{ onBlur: ({ value }) => required('A file')(value) }}>
                {(f) => (
                  <Field label="File" error={fieldError(f.state.meta)}
                    help="The first sheet is used unless you pick another later.">
                    <TextInput value={f.state.value} onBlur={f.handleBlur}
                      placeholder="finance/budget-2026.xlsx"
                      onChange={(e) => f.handleChange(e.currentTarget.value)} />
                  </Field>
                )}
              </form.Field>
            )}
          </FormSection>
        )}

        {step === 2 && spec && (
          <FormSection title="Test the connection"
            description="Better to find a wrong password now than in a scheduled run at 6am.">
            <div className={`flex items-center justify-between gap-4 rounded-lg border p-5
              ${test.status === 'ok'
                ? 'border-solid border-good bg-sunken'
                : 'border-dashed border-line bg-sunken'}`}>
              {test.status === 'idle' && (
                <>
                  <p className="text-small text-ink-secondary">Nothing has been contacted yet.</p>
                  <button type="button" className="cursor-pointer rounded-md border border-line bg-surface px-4 py-2 text-small text-ink hover:border-accent" onClick={runTest}>
                    Test connection
                  </button>
                </>
              )}
              {test.status === 'running' && <p className="text-small text-ink-secondary">Connecting…</p>}
              {test.status === 'ok' && (
                <>
                  <p className="text-small text-ink-secondary">
                    <strong>Connected.</strong> Found {test.tables} tables cronos can read.
                  </p>
                  <button type="button" className="cursor-pointer rounded-md border border-line bg-surface px-4 py-2 text-small text-ink hover:border-accent" onClick={runTest}>Test again</button>
                </>
              )}
              {test.status === 'failed' && (
                <>
                  <p className="text-small text-ink-secondary">{test.message}</p>
                  <button type="button" className="cursor-pointer rounded-md border border-line bg-surface px-4 py-2 text-small text-ink hover:border-accent" onClick={runTest}>Try again</button>
                </>
              )}
            </div>
            {spec.pushdown !== 'full' && (
              <Callout>
                <strong>{spec.pushdownLabel}.</strong> {spec.pushdownHint}
              </Callout>
            )}
          </FormSection>
        )}

        {step === 3 && (
          <FormSection title="Name this source"
            description="Report authors will pick it from a list by this name.">
            <form.Field name="name"
              validators={{ onBlur: ({ value }) => required('A name')(value) }}>
              {(f) => (
                <Field label="Name" error={fieldError(f.state.meta)}>
                  <TextInput value={f.state.value} onBlur={f.handleBlur}
                    placeholder="Production warehouse"
                    onChange={(e) => {
                      f.handleChange(e.currentTarget.value)
                      if (!slugEdited) form.setFieldValue('slug', toSlug(e.currentTarget.value))
                    }} />
                </Field>
              )}
            </form.Field>

            <form.Field name="slug" validators={{ onBlur: ({ value }) => slug(value) }}>
              {(f) => (
                <IdentifierField value={f.state.value} onBlur={f.handleBlur}
                      error={fieldError(f.state.meta)} 
                      usedFor="Datasets point at this name when they say which source they query."
                      onChange={(v) => { setSlugEdited(true); f.handleChange(v) }} />
              )}
            </form.Field>
          </FormSection>
        )}
      </Wizard>

      <button type="button" onClick={onCancel}
        className="mt-4 cursor-pointer p-0 text-small text-ink-muted underline">Cancel</button>
    </form>
  )
}

/** Pushdown capability, coloured by how much of a filter the source absorbs. */
function pushdownTone(p: string): string {
  if (p === 'full') return 'text-delta-good'
  if (p === 'partial') return 'text-ink-secondary'
  return 'text-serious'
}

type Values = { host: string; database: string; user: string; uri: string; endpoint: string; filePath: string }

function connectionComplete(spec: { shape: string } | null, v: Values): boolean {
  if (!spec) return false
  switch (spec.shape) {
    case 'sql': return !!(v.host && v.database && v.user)
    case 'object': return !!v.uri
    case 'api': return !!v.endpoint
    case 'file': return !!v.filePath
    default: return false
  }
}
