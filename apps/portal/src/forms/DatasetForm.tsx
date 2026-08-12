import { useMemo, useState } from 'react'
import { useForm, useStore } from '@tanstack/react-form'
import { Button, Select, Textarea, TextInput } from '@mantine/core'
import { Field, fieldError } from '../components/form/Field'
import { IdentifierField } from '../components/form/IdentifierField'
import { FormActions } from '../components/form/FormShell'
import { QueryBuilder } from '../components/query/QueryBuilder'
import { SqlView } from '../components/query/SqlView'
import { FieldsEditor } from './FieldsEditor'
import { ParamsEditor } from './ParamsEditor'
import { DataTable } from '../components/DataTable'
import { emptyQuery, fieldsFromQuery, toSql, type QueryModel } from '../lib/queryModel'
import { dataset, withCarry, type DatasetInput, type Loaded } from '../lib/definitions'
import { usePublish } from '../lib/usePublish'
import { PublishError } from '../components/form/PublishError'
import { UnmodelledWarning } from '../components/form/UnmodelledWarning'
import { sampleRows } from '../lib/sampleRows'
import type { Field as FieldDef, Param } from '../lib/types'
import { required, slug, toSlug } from '../lib/validators'

interface Props {
  onDone: () => void
  onCancel: () => void
  /** An existing dataset to edit. Absent means a new one. */
  initial?: Loaded<DatasetInput>
}

type Tab = 'query' | 'params' | 'fields' | 'security'
type Mode = 'visual' | 'sql'

const CARD = 'rounded-lg border border-line bg-surface p-5 shadow-card'

const SOURCES = [
  { value: 'warehouse', label: 'Production warehouse' },
  { value: 'events-lake', label: 'Event lake' },
  { value: 'billing-api', label: 'Billing API' },
]

/**
 * A dataset: a query against one source, plus the contract it exposes.
 *
 * The chain is DataSource → Dataset → Report. A dataset is where the SQL lives
 * and where governance is stated once — labels, types, roles, row-level
 * security — so that everything downstream inherits it instead of restating it.
 * See docs/report-format.md.
 */
export function DatasetForm({ onDone, onCancel, initial }: Props) {
  const stored = initial?.input
  /* Once someone edits the API name, typing in Name must stop
     overwriting it — silently discarding a deliberate edit. */
  const [slugEdited, setSlugEdited] = useState(false)
  const [tab, setTab] = useState<Tab>('query')
  /* Editing opens in SQL, because the visual model cannot be recovered from
     arbitrary SQL and a builder showing an empty canvas over a real query
     would be lying about what the dataset does. */
  const [mode, setMode] = useState<Mode>(stored ? 'sql' : 'visual')
  const [query, setQuery] = useState<QueryModel>(emptyQuery)
  const [rawSql, setRawSql] = useState(stored?.query ?? '')
  /* Author edits, kept separately so regenerating the query does not wipe the
     labels somebody has already fixed. */
  const [overrides, setOverrides] = useState<Record<string, Partial<FieldDef>>>({})
  const [rls, setRls] = useState(stored?.predicate ?? '')
  const [params, setParams] = useState<Param[]>(stored?.params ?? [])

  const { publish, error: publishError, busy } = usePublish()

  const form = useForm({
    defaultValues: {
      name: stored?.name ?? '', slug: stored?.slug ?? '',
      description: stored?.description ?? '', source: stored?.source ?? 'warehouse',
    },
    onSubmit: async ({ value }) => {
      const saved = await publish(withCarry(dataset({
        name: value.name, slug: value.slug, description: value.description,
        source: value.source, query: mode === 'sql' ? rawSql : generated,
        fields, params, predicate: rls,
      }), initial))
      if (saved) onDone()
    },
  })
  const values = useStore(form.store, (s) => s.values)

  const generated = toSql(query)

  /* Derived from the query, then layered with whatever the author has renamed
     or reclassified — regenerating the query must not discard those edits. */
  const fields: FieldDef[] = useMemo(() => {
    // A loaded dataset has no visual model, so its fields come from the file
    // it was read from — and stay editable, because overrides layer over both.
    const derived = stored && query.columns.length === 0
      ? stored.fields.map((f) => ({ ...f }))
      : fieldsFromQuery(query)
    for (const f of derived) {
      const edit = overrides[f.name]
      if (edit) Object.assign(f, edit)
    }
    return derived
  }, [query, overrides, stored])

  const preview = useMemo(() => sampleRows(fields.slice(0, 7)), [fields])

  const ready = values.name.trim() !== '' &&
    (mode === 'sql' ? rawSql.trim() !== '' : query.columns.length > 0)

  /* One-way by design: the visual model cannot be recovered from arbitrary SQL,
     so the switch says so before it happens rather than losing work quietly. */
  function switchToSql() {
    setRawSql(generated)
    setMode('sql')
  }

  const TABS: { id: Tab; label: string; count?: number }[] = [
    { id: 'query', label: 'Query' },
    { id: 'params', label: 'Parameters', count: params.length },
    { id: 'fields', label: 'Fields', count: fields.length },
    { id: 'security', label: 'Security' },
  ]

  return (
    <form onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); form.handleSubmit() }}>
      <div className="grid items-start gap-4 xl:grid-cols-[340px_minmax(0,1fr)]">
        {/* -- Settings rail ------------------------------------------------ */}
        <div className="grid gap-4 xl:sticky xl:top-[4.5rem]">
          <section className={CARD}>
            <h2 className="mb-4 text-lead font-semibold text-ink">Dataset</h2>
            <div className="grid gap-4">
              <form.Field name="name" validators={{ onBlur: ({ value }) => required('A name')(value) }}>
                {(f) => (
                  <Field label="Name" error={fieldError(f.state.meta)}>
                    <TextInput value={f.state.value} onBlur={f.handleBlur} placeholder="Invoices"
                      onChange={(e) => {
                        f.handleChange(e.currentTarget.value)
                        if (!slugEdited) form.setFieldValue('slug', toSlug(e.currentTarget.value))
                      }} />
                  </Field>
                )}
              </form.Field>

              <form.Field name="description">
                {(f) => (
                  <Field label="Description" required={false}
                    help="Report authors see this when picking a dataset.">
                    <Textarea autosize minRows={2} value={f.state.value} onBlur={f.handleBlur}
                      placeholder="Issued invoices with customer detail."
                      onChange={(e) => f.handleChange(e.currentTarget.value)} />
                  </Field>
                )}
              </form.Field>

              <form.Field name="slug" validators={{ onBlur: ({ value }) => slug(value) }}>
                {(f) => (
                  <IdentifierField value={f.state.value} onBlur={f.handleBlur}
                      error={fieldError(f.state.meta)} 
                      usedFor="Reports point at this name when they say which dataset they read."
                      fixed={!!stored}
                      onChange={(v) => { setSlugEdited(true); f.handleChange(v) }} />
                )}
              </form.Field>
            </div>
          </section>

          <section className={CARD}>
            <h2 className="text-lead font-semibold text-ink">Source</h2>
            <p className="mt-1 mb-4 text-small text-ink-secondary">
              Which connection this query runs against.
            </p>
            <form.Field name="source">
              {(f) => (
                <Field label="Data source">
                  <Select data={SOURCES} value={f.state.value} allowDeselect={false}
                    onChange={(v) => f.handleChange(v ?? 'warehouse')} />
                </Field>
              )}
            </form.Field>
          </section>

          <section className={CARD}>
            <h2 className="mb-3 text-lead font-semibold text-ink">Summary</h2>
            <dl className="grid gap-2 text-small">
              <Row term="Columns" value={String(query.columns.length)} />
              <Row term="Joined tables" value={String(query.joins.length)} />
              <Row term="Fields exposed" value={String(fields.filter((f) => !f.hidden).length)} />
              <Row term="Row-level security" value={rls ? 'On' : 'None'} />
            </dl>
          </section>
        </div>

        {/* -- Canvas ------------------------------------------------------- */}
        <section className="flex min-h-[calc(100vh-11rem)] flex-col rounded-lg border
                            border-line bg-surface shadow-card">
          <div className="flex gap-1 overflow-x-auto border-b border-line px-3" role="tablist">
            {TABS.map((t) => (
              <button key={t.id} type="button" role="tab" aria-selected={tab === t.id}
                onClick={() => setTab(t.id)}
                className={`shrink-0 cursor-pointer border-b-2 px-3 py-3 text-small font-medium
                  ${tab === t.id
                    ? 'border-accent text-ink'
                    : 'border-transparent text-ink-secondary hover:text-ink'}`}>
                {t.label}
                {t.count !== undefined && (
                  <span className="ml-1.5 text-caption text-ink-muted">{t.count}</span>
                )}
              </button>
            ))}
          </div>

          <div className="flex flex-1 flex-col gap-4 p-5">
            {tab === 'query' && (
              <>
                <div className="flex items-center justify-between gap-3">
                  <div className="inline-flex rounded-md border border-line p-0.5">
                    <ModeButton on={mode === 'visual'} onClick={() => setMode('visual')}>
                      Build it
                    </ModeButton>
                    <ModeButton on={mode === 'sql'} onClick={switchToSql}>Write SQL</ModeButton>
                  </div>
                  {mode === 'sql' && (
                    <Button variant="subtle" size="xs" color="gray"
                      onClick={() => { setMode('visual'); setRawSql('') }}>
                      Back to the builder
                    </Button>
                  )}
                </div>

                {mode === 'visual' ? (
                  <>
                    <QueryBuilder query={query} onChange={setQuery} />
                    {query.from && (
                      <div>
                        <h3 className="mb-2 text-small font-semibold text-ink">
                          The SQL this produces
                        </h3>
                        <SqlView sql={generated} />
                        <p className="mt-2 text-small text-ink-muted">
                          Highlighted values are bound at run time, not pasted into the query.
                        </p>
                      </div>
                    )}
                  </>
                ) : (
                  <>
                    <p className="rounded-r-md border-l-2 border-serious bg-sunken px-4 py-3
                                  text-small text-ink-secondary">
                      <strong className="text-ink">Once you edit this, the builder is gone.</strong>{' '}
                      cronos cannot turn arbitrary SQL back into steps. Going back to the
                      builder starts a fresh query.
                    </p>
                    <Textarea autosize minRows={14} value={rawSql} spellCheck={false}
                      aria-label="SQL" classNames={{ input: 'font-mono text-caption' }}
                      onChange={(e) => setRawSql(e.currentTarget.value)} />
                    <p className="text-small text-ink-muted">
                      Use <code className="font-mono">{'{{ .params.name }}'}</code> for parameters
                      and <code className="font-mono">{'{{ .scope.name }}'}</code> for row scope.
                      They are bound, never interpolated.
                    </p>
                  </>
                )}

                {(query.columns.length > 0 || rawSql) && (
                  <div className="mt-auto">
                    <h3 className="mb-2 text-small font-semibold text-ink">Sample rows</h3>
                    <div className="overflow-hidden rounded-lg border border-line">
                      <DataTable fields={fields.slice(0, 7)} rows={preview}
                        totalLabel="First 50 rows — the full query runs when a report uses it" />
                    </div>
                  </div>
                )}
              </>
            )}

            {tab === 'params' && (
              <>
                <p className="text-small text-ink-secondary">
                  What a report may ask this dataset. Each one reaches the query as a
                  bind argument — <code className="font-mono text-caption">{'{{ .params.name }}'}</code> —
                  never as SQL text.
                </p>
                <div className="mt-4">
                  <ParamsEditor params={params} onChange={setParams} />
                </div>
              </>
            )}

            {tab === 'fields' && (
              <>
                <p className="text-small text-ink-secondary">
                  What report authors will see. Labels here become the names in every
                  chart, filter and column downstream.
                </p>
                <FieldsEditor fields={fields}
                  onChange={(next) => setOverrides(Object.fromEntries(
                    next.map((f) => [f.name, f]),
                  ))} />
              </>
            )}

            {tab === 'security' && (
              <>
                <h3 className="text-body font-semibold text-ink">Row-level security</h3>
                <p className="max-w-[70ch] text-small text-ink-secondary">
                  Applied to every read of this dataset — previews, exports, embeds and
                  scheduled runs alike. There is no way to switch it off for one report.
                </p>
                <Field label="Only return rows where" required={false}
                  help="Leave empty for an internal dataset that project members may read in full.">
                  <TextInput value={rls} placeholder="customer_id = {{ .scope.customer_id }}"
                    classNames={{ input: 'font-mono text-caption' }}
                    onChange={(e) => setRls(e.currentTarget.value)} />
                </Field>
                <p className="max-w-[70ch] rounded-r-md border-l-2 border-serious bg-sunken
                              px-4 py-3 text-small text-ink-secondary">
                  <strong className="text-ink">Scope fails closed.</strong> If a caller has no{' '}
                  <code className="font-mono">.scope</code> value this matches nothing rather than
                  everything — so a dataset used by a <em>schedule</em> must not carry a scope
                  predicate, or the run will silently deliver zero documents.
                </p>
              </>
            )}
          </div>
        </section>
      </div>

      <UnmodelledWarning paths={initial?.drops} />
      <PublishError message={publishError} />

      <form.Subscribe selector={(s) => [s.isSubmitting] as const}>
        {([isSubmitting]) => (
          <FormActions canSubmit={ready} isSubmitting={isSubmitting || busy}
            submitLabel={stored ? 'Save dataset' : 'Create dataset'} onCancel={onCancel}
            hint={ready
              ? `${fields.filter((f) => !f.hidden).length} fields will be available to reports.`
              : 'Name the dataset and return at least one column to continue.'} />
        )}
      </form.Subscribe>
    </form>
  )
}

function ModeButton({
  on, onClick, children,
}: { on: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" onClick={onClick} aria-pressed={on}
      className={`cursor-pointer rounded-sm px-3 py-1.5 text-small font-medium
        ${on ? 'bg-accent-wash text-ink' : 'text-ink-secondary hover:text-ink'}`}>
      {children}
    </button>
  )
}

function Row({ term, value }: { term: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <dt className="text-ink-secondary">{term}</dt>
      <dd className="font-medium text-ink tabular-nums">{value}</dd>
    </div>
  )
}
