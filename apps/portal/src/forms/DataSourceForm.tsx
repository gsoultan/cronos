import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  connectionFor, dataSource, passwordSecret, withCarry, type Loaded, type SourceInput,
} from '../lib/definitions'
import { ApiError, connected, setSecret, testDataSource } from '../lib/api'
import { usePublish } from '../lib/usePublish'
import { useSecrets } from '../lib/useSecrets'
import {
  envName, reference, referenced, secretNameProblem, secretValueProblem, sourceSecret,
} from '../lib/secrets'
import { PublishError } from '../components/form/PublishError'
import { UnmodelledWarning } from '../components/form/UnmodelledWarning'
import { useForm, useStore } from '@tanstack/react-form'
import { NumberInput, PasswordInput, SegmentedControl, TextInput } from '@mantine/core'
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

/**
 * How a bucket's credentials are given: pasted here and stored as a secret,
 * or named — a secret somebody manages elsewhere, or `chain`.
 */
type CredentialMode = 'paste' | 'reference'

const CREDENTIAL_MODES = [
  { value: 'paste', label: 'Paste them' },
  { value: 'reference', label: 'Name a secret' },
]

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
 *
 * A password typed here is stored as a secret on the server, and the file
 * names that secret. It used to be collected and thrown away while the file
 * named a secret nothing held, so every database connected through the portal
 * had a connection string that could not open.
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
  /* Credentials this wizard stored earlier are named after the source, and
     reopen as a blank field that keeps them. Anything else in the file — a
     secret managed elsewhere, or `chain` — reopens as the reference it is. */
  const [credentialMode, setCredentialMode] = useState<CredentialMode>(
    stored?.credentials && referenced(stored.credentials) !== sourceSecret(stored.slug, 'credentials')
      ? 'reference' : 'paste')
  const [secretError, setSecretError] = useState<string | null>(null)
  const [sending, setSending] = useState(false)

  const { publish, error: publishError, busy } = usePublish()
  const queries = useQueryClient()
  /* Whether this deployment can keep a secret at all. Undefined until the
     server says, and then the form offers to store one and lets the server
     answer — a refusal nobody gave is not a reason to hide the field. */
  const { storing } = useSecrets()
  // A server that cannot store credentials can only be told where they are.
  const mode: CredentialMode = storing === false ? 'reference' : credentialMode

  const form = useForm({
    defaultValues: {
      host: stored?.host ?? '', port: stored?.port ?? 5432,
      database: stored?.database ?? '', user: stored?.user ?? '',
      // Never loaded, because no request returns a secret's value: the file
      // holds a ${secret:…} reference and the server keeps what is behind it.
      // Left blank, an edit keeps the password in use.
      password: '',
      // The connection string as stored, for the shapes that show it. Kept
      // verbatim: a DSN somebody wrote is theirs, and recomposing it from parts
      // is how a query parameter goes missing.
      dsn: stored?.dsn ?? '',
      uri: stored?.uri ?? '', endpoint: '',
      // A store's own address, kept apart from the API shape's `endpoint`:
      // that one is folded into `uri` on submit, and a MinIO address landing
      // there would become the location of the lake.
      storeEndpoint: stored?.storeEndpoint ?? '',
      region: stored?.region ?? '',
      // The reference, not the credential. Reloaded because the file holds
      // ${secret:…} and blanking it on an edit would drop it.
      credentials: stored?.credentials ?? '',
      // Pasted pairs, stored as a secret on save. Blank like the password, for
      // the same reason.
      credentialsValue: '',
      filePath: stored?.filePath ?? '',
      name: stored?.name ?? '', slug: stored?.slug ?? '',
    },
    onSubmit: async ({ value }) => {
      /*
         The secret first, then the definition that names it.

         Publishing a source opens it there and then, with whatever its
         references resolve to at that moment. Stored the other way round, a
         new source is opened without its password, fails, and waits for
         something to reopen it.
      */
      const secret = secretFor(value)
      if (secret && !(await store(secret.name, secret.value))) return

      const saved = await publish(withCarry(dataSource({
        name: value.name, slug: value.slug, kind: kind ?? 'postgres',
        host: value.host, port: value.port, database: value.database,
        user: value.user, uri: value.uri || value.endpoint, filePath: value.filePath,
        region: value.region, storeEndpoint: value.storeEndpoint,
        credentials: credentialsFor(value),
        // Both read from the stored string, so one rebuilt from new parts
        // names the same secret and keeps its ?sslmode.
        passwordSecret: stored?.passwordSecret,
        dsnOptions: stored?.dsnOptions,
        /*
           The connection string: typed, kept, or built — see connectionFor.

           Typed where the form shows one. That half was already here, with a
           comment observing that a SQLite DSN has no host; what was missing was
           any way to see or change it, so a SQLite source opened a form asking
           for a port. */
        dsn: connectionFor(spec?.shape ?? 'sql', { ...value, password: typedPassword(value) }, stored),
      }), initial), initial?.version)
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

  /** The secret the password is stored under, once the source has a name. */
  const passwordName = (v: { slug: string }) =>
    passwordSecret({ slug: v.slug, passwordSecret: stored?.passwordSecret })

  /* The password this save stores: what was typed, unless the server has said
     it cannot keep one — the field is gone by then, and a value typed before
     the answer arrived is not a reason to refuse the save with a 503. */
  const typedPassword = (v: { password: string }) => (storing === false ? '' : v.password)

  /**
   * The secret this save stores before publishing, if anything typed becomes one.
   *
   * As typed, both of them. A connection string that reads the password inside
   * a URL says so — `${secret:name|url}` — and the server encodes it there, so
   * what is stored is the password itself.
   */
  function secretFor(v: typeof values): { name: string; value: string } | null {
    if (spec?.shape === 'sql' && typedPassword(v) !== '') {
      return { name: passwordName(v), value: typedPassword(v) }
    }
    if (spec?.shape === 'object' && mode === 'paste' && v.credentialsValue !== '') {
      return { name: sourceSecret(v.slug, 'credentials'), value: v.credentialsValue }
    }
    return null
  }

  /**
   * What the file's `credentials` says.
   *
   * A reference to what was pasted; the reference typed; or, when nothing was
   * pasted, what the source already had — an edit keeps its credentials the
   * way it keeps its password, and a new source with none is a public bucket.
   */
  function credentialsFor(v: typeof values): string {
    if (spec?.shape !== 'object' || mode === 'reference') return v.credentials
    if (v.credentialsValue !== '') return reference(sourceSecret(v.slug, 'credentials'))
    return stored?.credentials ?? ''
  }

  /**
   * Stores one secret, and says why not in the server's words.
   *
   * Unconnected it stores nothing and reports success, as publishing does:
   * sample mode has nowhere to keep a password, and a wizard that refused to
   * finish would be untestable before a server exists.
   */
  async function store(name: string, secretValue: string): Promise<boolean> {
    if (!connected()) return true
    const problem = secretNameProblem(name) ?? secretValueProblem(secretValue)
    if (problem) {
      setSecretError(`The secret ${name} cannot be stored: ${problem.toLowerCase()}.`)
      return false
    }
    setSending(true)
    setSecretError(null)
    try {
      await setSecret(name, secretValue)
      await queries.invalidateQueries({ queryKey: ['secrets'] })
      return true
    } catch (err) {
      setSecretError(err instanceof ApiError ? err.message : 'Could not reach the server.')
      return false
    } finally {
      setSending(false)
    }
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
   * password it resolves — the test never sends one, because a definition is a
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

  /* What this save will store, named — shown on the last step, where the name
     it is stored under has just been decided. */
  const toStore = secretFor(values)

  return (
    <form onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); advance() }}>
      <Wizard
        steps={STEPS} current={step} completed={completed}
        onStep={setStep} onBack={() => setStep((s) => s - 1)} onNext={advance}
        canAdvance={canAdvance} busy={busy || sending || test.status === 'running'}
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
                        placeholder="db.internal.acme.com" data-testid="source-host"
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>

                <form.Field name="port" validators={{ onBlur: ({ value }) => portRule(value) }}>
                  {(f) => (
                    <Field label="Port" error={fieldError(f.state.meta)}>
                      <NumberInput value={f.state.value} onBlur={f.handleBlur} w={160}
                        data-testid="source-port"
                        onChange={(v) => f.handleChange(Number(v) || 0)} />
                    </Field>
                  )}
                </form.Field>

                <form.Field name="database" validators={{ onBlur: ({ value }) => required('A database')(value) }}>
                  {(f) => (
                    <Field label="Database" error={fieldError(f.state.meta)}>
                      <TextInput value={f.state.value} onBlur={f.handleBlur}
                        data-testid="source-database"
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>

                <form.Field name="user" validators={{ onBlur: ({ value }) => required('A username')(value) }}>
                  {(f) => (
                    <Field label="Username" error={fieldError(f.state.meta)}
                      help="Give it read-only access. cronos never writes to your data.">
                      <TextInput value={f.state.value} onBlur={f.handleBlur}
                        data-testid="source-user"
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>

                {/* Replaced rather than disabled where nothing can be stored:
                    a field that takes a password and keeps it nowhere is the
                    bug this form had, and a greyed one reads as a bug too. */}
                {storing === false ? (
                  <Callout>
                    <span data-testid="password-environment">
                      <strong>This server cannot store a password</strong> — it has no
                      CRONOS_SECRETS_KEY. Set it in the server’s environment instead, as{' '}
                      {values.slug
                        ? <code className="font-mono text-caption">{envName(passwordName(values))}</code>
                        : 'a variable named after this source (the last step says which)'}.
                    </span>
                  </Callout>
                ) : (
                  <form.Field name="password">
                    {(f) => (
                      <Field label="Password" required={!stored}
                        help={stored
                          ? <>Leave blank to keep the one in use. A new one replaces the secret{' '}
                              <code className="font-mono text-caption">{passwordName(values)}</code>{' '}
                              when you save.</>
                          : 'Stored on the server as a secret when you save, named after this source. '
                            + 'The definition names the secret and never holds the password.'}>
                        {/* new-password, so the browser does not offer the
                            password somebody signs in to cronos with. */}
                        <PasswordInput value={f.state.value} onBlur={f.handleBlur}
                          autoComplete="new-password" data-1p-ignore data-lpignore="true"
                          data-testid="source-password" placeholder={stored ? 'Unchanged' : undefined}
                          onChange={(e) => f.handleChange(e.currentTarget.value)} />
                      </Field>
                    )}
                  </form.Field>
                )}
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
              <>
                <form.Field name="uri" validators={{ onBlur: ({ value }) => required('A location')(value) }}>
                  {(f) => (
                    <Field label="Location" error={fieldError(f.state.meta)}
                      help="A bucket and prefix. Files underneath are read as one table.">
                      <TextInput value={f.state.value} onBlur={f.handleBlur}
                        data-testid="source-uri"
                        placeholder="s3://acme-lake/events/"
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>
                {/*
                   A private bucket needs a key, and until there was anywhere to
                   put one the only way to define a readable lake was to edit
                   the YAML by hand — which is where somebody pastes the key
                   itself rather than a reference to it.

                   Two ways in now. Pasted, the pairs are stored as a secret
                   named after the source and the file names that; named, the
                   file carries the reference somebody typed, for a secret
                   managed elsewhere, or `chain` for the machine's own role.
                */}
                <Field label="Credentials" required={false} help={credentialsHelp(mode, !!stored, storing)}>
                  <div className="grid gap-2">
                    {storing !== false && (
                      <SegmentedControl size="xs" value={mode} data={CREDENTIAL_MODES}
                        data-testid="credentials-mode" className="justify-self-start"
                        onChange={(v) => setCredentialMode(v as CredentialMode)} />
                    )}
                    {mode === 'paste' ? (
                      <form.Field name="credentialsValue">
                        {(f) => (
                          <PasswordInput value={f.state.value} onBlur={f.handleBlur}
                            autoComplete="new-password" data-1p-ignore data-lpignore="true"
                            data-testid="source-credentials-value"
                            aria-label="Credentials to store"
                            placeholder={stored?.credentials ? 'Unchanged' : 'key_id=…;secret=…'}
                            onChange={(e) => f.handleChange(e.currentTarget.value)} />
                        )}
                      </form.Field>
                    ) : (
                      <form.Field name="credentials">
                        {(f) => (
                          <TextInput value={f.state.value} onBlur={f.handleBlur}
                            data-testid="source-credentials" aria-label="Credentials reference"
                            placeholder="${secret:lake_creds}"
                            onChange={(e) => f.handleChange(e.currentTarget.value)} />
                        )}
                      </form.Field>
                    )}
                  </div>
                </Field>
                <form.Field name="region">
                  {(f) => (
                    <Field label="Region" required={false}
                      help="For S3 and its compatibles. Azure resolves one from the account and takes none.">
                      <TextInput value={f.state.value} onBlur={f.handleBlur}
                        data-testid="source-region"
                        placeholder="eu-central-1"
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>
                <form.Field name="storeEndpoint"
                  validators={{ onBlur: ({ value }) => (value ? url(value) : undefined) }}>
                  {(f) => (
                    <Field label="Endpoint" required={false} error={fieldError(f.state.meta)}
                      help="Only for a store that is not the cloud's own — MinIO, Ceph, an appliance. The scheme decides TLS.">
                      <TextInput value={f.state.value} onBlur={f.handleBlur}
                        data-testid="source-endpoint"
                        placeholder="http://minio.internal:9000"
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>
              </>
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
                {/*
                   There was an "Authorisation header" field here, and what was
                   typed into it went nowhere: the definition has no place for a
                   header, and the server reads an http(s) address with no
                   credential at all — it refuses one on it. Storing the token
                   as a secret now would only store something nothing reads, so
                   the form says what is true instead of taking a token it
                   cannot use.
                */}
                <Callout>
                  <span data-testid="api-no-header">
                    cronos requests this address as it is, with no authorisation header — a
                    source has nowhere to keep one — so the endpoint has to answer without a
                    token.
                  </span>
                </Callout>
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
            {/* The probe reads what the server holds now. A password typed on
                this edit is not stored until Save, so a test before then is a
                test of the old one — said, so a pass is not read as the new
                password working. */}
            {stored && toStore && (
              <Callout>
                The test uses what the server holds now. What you typed is stored when you
                save — test again from the Data page afterwards.
              </Callout>
            )}
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
                    placeholder="Production warehouse" data-testid="source-name"
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

            {/* Named here because this is where the name is decided: the
                secret is called after the source, and somebody who manages
                secrets elsewhere will look for it by that name. */}
            {toStore && values.slug && (
              <p className="text-caption text-ink-muted" data-testid="stored-as">
                What you typed is stored as the secret{' '}
                <code className="font-mono text-ink-secondary">{toStore.name}</code> when you save,
                and the definition names it.
              </p>
            )}
            {storing === false && spec?.shape === 'sql' && values.slug && (
              <Callout>
                <span data-testid="password-environment-name">
                  Before this source can connect, set{' '}
                  <code className="font-mono text-caption">{envName(passwordName(values))}</code>{' '}
                  to its password in the server’s environment. This server
                  cannot store one: it has no CRONOS_SECRETS_KEY.
                </span>
              </Callout>
            )}
          </FormSection>
        )}
      </Wizard>

      <UnmodelledWarning paths={initial?.drops} />
      <PublishError message={secretError ?? publishError} />

      <button type="button" onClick={onCancel}
        className="mt-4 cursor-pointer p-0 text-small text-ink-muted underline">Cancel</button>
    </form>
  )
}

/**
 * What the credentials field takes, for the way it is being given.
 *
 * The pair names are the store's own — key_id and secret for the S3 API,
 * account_name and account_key for Azure — because a definition that renames
 * what a cloud console shows is one somebody has to translate at the moment
 * they are least able to. definition/credentials.go is the list.
 */
function credentialsHelp(mode: CredentialMode, editing: boolean, storing: boolean | undefined): string {
  if (storing === false) {
    return 'This server cannot store credentials — it has no CRONOS_SECRETS_KEY. Name a secret '
      + 'its environment holds, as ${secret:name}, or write chain for the machine’s own role. '
      + 'Leave blank for a public bucket.'
  }
  if (mode === 'reference') {
    return 'A secret you manage elsewhere, as ${secret:name} — or chain, for the machine’s own '
      + 'role. Leave blank for a public bucket.'
  }
  return 'key_id=…;secret=… for S3, GCS and R2; account_name=…;account_key=… for Azure. Stored on '
    + 'the server as a secret when you save — the definition names it and never holds a key. '
    + (editing ? 'Leave blank to keep what it reads now.' : 'Leave blank for a public bucket.')
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
  region: string; storeEndpoint: string; credentials: string
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
