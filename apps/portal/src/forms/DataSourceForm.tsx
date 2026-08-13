import { useState } from 'react'
import { dataSource, withCarry, type Loaded, type SourceInput } from '../lib/definitions'
import { ApiError, testDataSource } from '../lib/api'
import { usePublish } from '../lib/usePublish'
import { PublishError } from '../components/form/PublishError'
import { UnmodelledWarning } from '../components/form/UnmodelledWarning'
import { useForm, useStore } from '@tanstack/react-form'
import { NumberInput, PasswordInput, TextInput, Textarea } from '@mantine/core'
import { Field, fieldError } from '../components/form/Field'
import { IdentifierField } from '../components/form/IdentifierField'
import { FormSection, Callout } from '../components/form/FormShell'
import { Wizard, type Step } from '../components/form/Wizard'
import { all, port as portRule, required, slug, toSlug, url } from '../lib/validators'
import { SOURCE_KINDS, type SourceKind, type SourceSpec } from '../lib/sources'

const STEPS: Step[] = [
  { id: 'kind', label: 'Choose a source', hint: 'Where the data lives' },
  { id: 'connect', label: 'Connect', hint: 'Address and credentials' },
  { id: 'test', label: 'Test', hint: 'Check it works before saving' },
  { id: 'name', label: 'Name it', hint: 'How your team will find it' },
]

type TestState = { status: 'idle' | 'running' | 'ok' | 'failed'; message?: string }

interface Props {
  onDone: () => void
  onCancel: () => void
  /** An existing source to edit. Absent means a new one. */
  initial?: Loaded<SourceInput>
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
export function DataSourceForm({ onDone, onCancel, initial }: Props) {
  const stored = initial?.input
  /* Once someone edits the API name, typing in Name must stop
     overwriting it — silently discarding a deliberate edit. */
  const [slugEdited, setSlugEdited] = useState(false)
  /* Editing starts on the connection step with every step behind it already
     reachable: the kind was chosen when the source was created, and asking for
     it again would imply changing it is what this screen is for. */
  const [step, setStep] = useState(stored ? 1 : 0)
  const [completed, setCompleted] = useState(stored ? STEPS.length - 1 : 0)
  const [kind, setKind] = useState<SourceKind | null>((stored?.kind as SourceKind) ?? null)
  const [test, setTest] = useState<TestState>({ status: 'idle' })

  const { publish, error: publishError, busy } = usePublish()

  const form = useForm({
    defaultValues: {
      host: stored?.host ?? '', port: stored?.port ?? 5432,
      database: stored?.database ?? '', user: stored?.user ?? '',
      // Never loaded, because it was never stored: the definition holds a
      // ${secret:…} reference and the server does not return the value behind
      // it. Left blank, an edit publishes the same reference again.
      password: '',
      // The connection string as stored, for the shapes that show it. Kept
      // verbatim: a DSN somebody wrote is theirs, and recomposing it from parts
      // is how a query parameter goes missing.
      dsn: stored?.dsn ?? '',
      uri: stored?.uri ?? '', endpoint: '', authHeader: '',
      filePath: stored?.filePath ?? '',
      name: stored?.name ?? '', slug: stored?.slug ?? '',
    },
    onSubmit: async ({ value }) => {
      const saved = await publish(withCarry(dataSource({
        name: value.name, slug: value.slug, kind: kind ?? 'postgres',
        host: value.host, port: value.port, database: value.database,
        user: value.user, uri: value.uri || value.endpoint, filePath: value.filePath,
        /*
           The connection string: edited, or kept.

           Edited where the form shows one — a SQLite path, or a driver this
           build has no screen for. Otherwise the stored one, while nothing
           about the connection was touched: not every DSN decomposes into host
           and database and user, so rebuilding one that was only ever displayed
           would replace a working connection with a guess.

           The second half was already here, with a comment observing that a
           SQLite DSN has no host. What was missing was any way to see or change
           it, so a SQLite source opened a form asking for a port. */
        dsn: value.dsn || (untouched(value) ? stored?.dsn : undefined),
      }), initial))
      if (saved) onDone()
    },
  })

  const values = useStore(form.store, (s) => s.values)
  /*
     The card for this kind, or one made up for a driver that has no card.

     This was `.find(...)!`, which tells the compiler a lookup always succeeds
     when it plainly does not: a source stored with `driver: sqlite` — which the
     engine supports and the demo ships — matched nothing, `spec` was undefined
     at runtime while typed as present, and every `step === 1 && spec` below went
     quietly false. Editing it showed three ticked steps, a highlighted Connect,
     and no fields at all, with Continue greyed out and Cancel the only way
     forward.

     Nothing about that is specific to sqlite. Any driver the engine gains
     before the picker does lands the same way, so the fallback is the fix and
     the new card is the smaller half of it. Treated as SQL because every driver
     without a card is one: it asks for a DSN and says which driver it is
     talking about rather than pretending to know more.
  */
  const spec: SourceSpec | null = kind
    ? SOURCE_KINDS.find((k) => k.id === kind) ?? unknownDriver(kind)
    : null

  /** Whether every connection field still holds what was loaded. */
  function untouched(v: { host: string; port: number; database: string; user: string; password: string }) {
    return !!stored && v.host === (stored.host ?? '') && v.port === (stored.port ?? 5432) &&
      v.database === (stored.database ?? '') && v.user === (stored.user ?? '') && v.password === ''
  }

  function advance() {
    if (step === STEPS.length - 1) return void form.handleSubmit()
    const next = step + 1
    setStep(next)
    setCompleted((c) => Math.max(c, next))
  }

  /*
   * A source can only be tested once it exists.
   *
   * The connection is opened by the server, from a definition it holds and a
   * password it resolves — the form never sends one, because a definition is a
   * file somebody commits and a secret in one is a secret in their git history
   * for ever. So there is nothing here for a probe to connect with until the
   * source has been saved.
   *
   * It used to wait nine hundred milliseconds and report twenty-four tables,
   * whatever had been typed. A test that cannot fail is worse than no test:
   * the step exists to catch a wrong password now rather than at 6am, and one
   * that always passes teaches somebody to trust it.
   */
  async function runTest() {
    if (!stored) {
      setTest({
        status: 'failed',
        message: 'This source has not been saved yet, so there is nothing to connect to. '
          + 'Save it, then test it from the Data page — the server opens the connection, '
          + 'using a password it resolves rather than one this form sends.',
      })
      setCompleted((c) => Math.max(c, 3))
      return
    }

    setTest({ status: 'running' })
    try {
      const probe = await testDataSource(stored.slug)
      setTest(probe.ok
        ? { status: 'ok', message: `Answered in ${probe.ms} ms.` }
        : { status: 'failed', message: probe.error ?? 'No answer.' })
    } catch (err) {
      setTest({
        status: 'failed',
        message: err instanceof ApiError ? err.message : 'Could not reach the server.',
      })
    }
    setCompleted((c) => Math.max(c, 3))
  }

  const canAdvance = (() => {
    switch (step) {
      case 0: return kind !== null
      case 1: return connectionComplete(spec, values)
      /* Not gated on a passing test. A new source cannot be tested until it
         exists, and refusing to advance would make the wizard unfinishable. */
      case 2: return test.status !== 'running'
      case 3: return values.name.trim().length > 1
      default: return false
    }
  })()

  return (
    <form onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); advance() }}>
      <Wizard
        steps={STEPS} current={step} completed={completed}
        onStep={setStep} onBack={() => setStep((s) => s - 1)} onNext={advance}
        canAdvance={canAdvance} busy={busy || test.status === 'running'}
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

            {spec.shape === 'dsn' && (
              <form.Field name="dsn"
                validators={{ onBlur: ({ value }) => required('A connection string')(value) }}>
                {(f) => (
                  <Field label="Connection string" error={fieldError(f.state.meta)}
                    help={'Passed to the driver as written. Use ${secret:name} rather than '
                      + 'a password here — a definition is a file somebody commits.'}>
                    <TextInput value={f.state.value as string} onBlur={f.handleBlur}
                      data-testid="source-dsn"
                      placeholder="file:/var/lib/cronos/warehouse.db"
                      onChange={(e) => f.handleChange(e.currentTarget.value)} />
                  </Field>
                )}
              </form.Field>
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
                  <p className="text-small text-ink-secondary">
                    {stored
                      ? 'Nothing has been contacted yet.'
                      : 'A source is contacted by the server, which cannot do it until this one is saved.'}
                  </p>
                  <button type="button" className="cursor-pointer rounded-md border border-line bg-surface px-4 py-2 text-small text-ink hover:border-accent" onClick={runTest}>
                    Test connection
                  </button>
                </>
              )}
              {test.status === 'running' && <p className="text-small text-ink-secondary">Connecting…</p>}
              {test.status === 'ok' && (
                <>
                  <p className="text-small text-ink-secondary" data-testid="probe-ok">
                    <strong>Connected.</strong> {test.message}
                  </p>
                  <button type="button" className="cursor-pointer rounded-md border border-line bg-surface px-4 py-2 text-small text-ink hover:border-accent" onClick={runTest}>Test again</button>
                </>
              )}
              {test.status === 'failed' && (
                <>
                  <p className="text-small text-ink-secondary" data-testid="probe-failed">
                    {test.message}
                  </p>
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
                      fixed={!!stored}
                      onChange={(v) => { setSlugEdited(true); f.handleChange(v) }} />
              )}
            </form.Field>
          </FormSection>
        )}
      </Wizard>

      <UnmodelledWarning paths={initial?.drops} />
      <PublishError message={publishError} />

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

type Values = {
  host: string; database: string; user: string
  dsn: string; uri: string; endpoint: string; filePath: string
}

function connectionComplete(spec: { shape: string } | null, v: Values): boolean {
  if (!spec) return false
  switch (spec.shape) {
    case 'sql': return !!(v.host && v.database && v.user)
    case 'dsn': return !!v.dsn
    case 'object': return !!v.uri
    case 'api': return !!v.endpoint
    case 'file': return !!v.filePath
    default: return false
  }
}

/**
 * A card for a driver the picker has never heard of.
 *
 * So that a definition remains editable rather than opening a form with nothing
 * in it. Named after the driver, because "Connect to sqlite" is what somebody
 * needs to read to know the page is about the thing they clicked — and the hint
 * says plainly that this is a driver without a dedicated screen, rather than
 * implying the fields shown are all there are.
 */
function unknownDriver(kind: string): SourceSpec {
  return {
    id: kind as SourceKind,
    label: kind,
    hint: 'A driver this version has no dedicated screen for',
    icon: '◇',
    shape: 'dsn',
    connectHint: `cronos can open ${kind}, and this build has no dedicated screen `
      + 'for it. The connection string is passed through as written.',
    pushdown: 'declared',
    pushdownLabel: 'Filters run where the driver puts them',
    pushdownHint: 'This build cannot say how much of a filter this driver pushes '
      + 'down. Check the source documentation before relying on it for a large table.',
  }
}
