import { useState } from 'react'
import { toSlug } from '../lib/validators'
import { schedule, withCarry, type Loaded, type ScheduleInput } from '../lib/definitions'
import { usePublish } from '../lib/usePublish'
import { PublishError } from '../components/form/PublishError'
import { UnmodelledWarning } from '../components/form/UnmodelledWarning'
import { useForm, useStore } from '@tanstack/react-form'
import { NumberInput, Select, Switch, TextInput } from '@mantine/core'
import { Field, fieldError } from '../components/form/Field'
import { FormActions, FormSection } from '../components/form/FormShell'
import { CRON_PRESETS, cronToText } from '../lib/cronText'
import { useDatasets, useReportChoices } from '../lib/useDatasets'
import { all, cron as cronRule, email, required } from '../lib/validators'

interface Props {
  onDone: () => void
  onCancel: () => void
  /** An existing schedule to edit. Absent means a new one. */
  initial?: Loaded<ScheduleInput>
}

const TIMEZONES = ['Europe/Berlin', 'Europe/London', 'America/New_York', 'Asia/Jakarta', 'UTC']

/**
 * Scheduling, including bursting.
 *
 * Two things carry the weight here: the timing is confirmed as a sentence
 * rather than as cron, and bursting is explained by what it does to the send
 * ("one document each, 812 in total") rather than by its name, which means
 * nothing to anyone outside this category.
 */
export function ScheduleForm({ onDone, onCancel, initial }: Props) {
  const stored = initial?.input
  const [burst, setBurst] = useState(!!stored?.burstDataset)
  const [channelId, setChannelId] = useState<'email' | 'telegram'>(
    stored?.channel === 'telegram' ? 'telegram' : 'email')

  const { publish, error: publishError, busy } = usePublish()

  const form = useForm({
    defaultValues: {
      report: stored?.report ?? '', output: stored?.output ?? 'pdf',
      cron: stored?.cron ?? '0 6 1 * *', timezone: stored?.timezone ?? 'Europe/Berlin',
      burstDataset: stored?.burstDataset ?? '', recipientField: stored?.recipientField ?? '',
      to: stored?.to ?? '', subject: stored?.subject ?? '',
      concurrency: stored?.concurrency ?? 8, retries: stored?.retries ?? 3,
      alert: stored?.alert ?? '',
    },
    onSubmit: async ({ value }) => {
      const saved = await publish(withCarry(schedule({
        // The stored name is kept: it is what selects the schedule this
        // overwrites, and deriving a new one from an edited report would
        // publish a second schedule beside the one being edited.
        name: stored?.name ?? value.report,
        slug: stored?.slug ?? toSlug(`${value.report}-${value.output}`),
        report: value.report, output: value.output,
        cron: value.cron, timezone: value.timezone,
        burstDataset: burst ? value.burstDataset : undefined,
        recipientField: burst ? value.recipientField : undefined,
        to: value.to, subject: value.subject, filename: stored?.filename,
        concurrency: value.concurrency, retries: value.retries, alert: value.alert,
      }), initial))
      if (saved) onDone()
    },
  })

  const v = useStore(form.store, (s) => s.values)
  /* The project's own reports and datasets. Both pickers listed a fixture's
     two entries until the portal was connected to its own API — see
     useDatasets. */
  const { datasets } = useDatasets()
  const { reports } = useReportChoices()
  const burstDataset = datasets.find((d) => d.name === v.burstDataset)
  const recipients = burst ? 812 : 1
  const ready = v.report !== '' && (burst ? !!v.burstDataset && !!v.recipientField : !!v.to)

  return (
    <form onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); form.handleSubmit() }}>
      <FormSection title="What should be sent?">
        <form.Field name="report" validators={{ onBlur: ({ value }) => required('A report')(value) }}>
          {(f) => (
            <Field label="Report" error={fieldError(f.state.meta)}>
              <Select data={reports.map((r) => ({ value: r.name, label: r.label }))}
                value={f.state.value || null} onBlur={f.handleBlur} allowDeselect={false}
                placeholder="Choose a report" onChange={(x) => f.handleChange(x ?? '')} />
            </Field>
          )}
        </form.Field>

        <form.Field name="output">
          {(f) => (
            <Field label="Format" help="Only formats this report produces are listed.">
              <Select allowDeselect={false} value={f.state.value}
                data={[
                  { value: 'pdf', label: 'PDF — print-ready pages' },
                  { value: 'xlsx', label: 'Excel — one row per record' },
                  { value: 'csv', label: 'CSV — raw data' },
                ]}
                onChange={(x) => f.handleChange(x ?? 'pdf')} />
            </Field>
          )}
        </form.Field>
      </FormSection>

      <FormSection title="When?"
        description="Pick a common pattern, or write the expression if you already think in cron.">
        <div className="flex flex-wrap gap-2">
          {CRON_PRESETS.map((p) => (
            <button key={p.cron} type="button"
              onClick={() => form.setFieldValue('cron', p.cron)}
              className={`cursor-pointer rounded-full border px-3 py-2 text-small text-ink
                hover:border-accent ${v.cron === p.cron
                  ? 'border-accent bg-accent-wash font-medium'
                  : 'border-line bg-sunken'}`}>
              {p.label}
            </button>
          ))}
        </div>

        <form.Field name="cron" validators={{ onBlur: ({ value }) => cronRule(value) }}>
          {(f) => (
            <Field label="Schedule" error={fieldError(f.state.meta)}>
              <TextInput value={f.state.value} onBlur={f.handleBlur} className="tabular" w={220}
                onChange={(e) => f.handleChange(e.currentTarget.value)} />
            </Field>
          )}
        </form.Field>

        {/* The sentence, not the expression, is what people check. */}
        <p className="flex items-center gap-2 rounded-md bg-accent-wash px-4 py-3 font-medium">
          <span aria-hidden>🕑</span>
          {cronToText(v.cron)} — {v.timezone}
        </p>

        <form.Field name="timezone">
          {(f) => (
            <Field label="Time zone"
              help="Relative dates like “last month” resolve against the run date in this zone.">
              <Select data={TIMEZONES} value={f.state.value} allowDeselect={false} searchable w={260}
                onChange={(x) => f.handleChange(x ?? 'UTC')} />
            </Field>
          )}
        </form.Field>
      </FormSection>

      <FormSection title="Who receives it?">
        <Switch checked={burst} onChange={(e) => setBurst(e.currentTarget.checked)}
          label="Send a personalised copy to each recipient"
          description="One run produces one document per row — each filtered to that recipient's own data."
          mb={16} />

        <Field label="Send it where" required={false}
          help="Telegram needs the bot to already be in the chat — see Settings → Channels.">
          <Select allowDeselect={false} value={channelId} w={220}
            data={[{ value: 'email', label: 'Email' }, { value: 'telegram', label: 'Telegram' }]}
            onChange={(next) => setChannelId((next ?? 'email') as 'email' | 'telegram')} />
        </Field>

        {burst ? (
          <>
            <form.Field name="burstDataset"
              validators={{ onBlur: ({ value }) => required('A recipient list')(value) }}>
              {(f) => (
                <Field label="Recipient list" error={fieldError(f.state.meta)}
                  help="A dataset with one row per recipient — usually customers or accounts.">
                  <Select data={datasets.map((d) => ({ value: d.name, label: d.label }))}
                    value={f.state.value || null} onBlur={f.handleBlur} allowDeselect={false}
                    placeholder="Choose a dataset" onChange={(x) => f.handleChange(x ?? '')} />
                </Field>
              )}
            </form.Field>

            {burstDataset && (
              <form.Field name="recipientField"
                validators={{ onBlur: ({ value }) => required('An email field')(value) }}>
                {(f) => (
                  <Field label="Email column" error={fieldError(f.state.meta)}
                    help="Which column holds the address to send each copy to.">
                    <Select
                      data={burstDataset.fields.filter((x) => !x.hidden)
                        .map((x) => ({ value: x.name, label: x.label }))}
                      value={f.state.value || null} onBlur={f.handleBlur} allowDeselect={false}
                      onChange={(x) => f.handleChange(x ?? '')} />
                  </Field>
                )}
              </form.Field>
            )}

            <form.Field name="concurrency">
              {(f) => (
                <Field label="Send at most" required={false}
                  help="Documents rendered at once. Higher is faster but heavier on your database.">
                  <NumberInput value={f.state.value} min={1} max={64} w={140} suffix=" at a time"
                    onChange={(x) => f.handleChange(Number(x) || 1)} />
                </Field>
              )}
            </form.Field>
          </>
        ) : (
          <form.Field name="to"
            validators={{ onBlur: ({ value }) => all(required('An address'), email)(value) }}>
            {(f) => (
              <Field label="Send to" error={fieldError(f.state.meta)}>
                <TextInput value={f.state.value} onBlur={f.handleBlur}
                  placeholder="finance@acme.com"
                  onChange={(e) => f.handleChange(e.currentTarget.value)} />
              </Field>
            )}
          </form.Field>
        )}
      </FormSection>

      <FormSection title="If something goes wrong"
        description="A partial failure still delivers the successes — retries apply per recipient, not per run.">
        <form.Field name="retries">
          {(f) => (
            <Field label="Retry" required={false}>
              <NumberInput value={f.state.value} min={0} max={10} w={140} suffix=" times"
                onChange={(x) => f.handleChange(Number(x) || 0)} />
            </Field>
          )}
        </form.Field>
        <form.Field name="alert" validators={{ onBlur: ({ value }) => email(value) }}>
          {(f) => (
            <Field label="Tell someone" required={false} error={fieldError(f.state.meta)}
              help="Emailed if a run fails after its retries.">
              <TextInput value={f.state.value} onBlur={f.handleBlur} placeholder="ops@acme.com"
                onChange={(e) => f.handleChange(e.currentTarget.value)} />
            </Field>
          )}
        </form.Field>
      </FormSection>

      <UnmodelledWarning paths={initial?.drops} />
      <PublishError message={publishError} />

      <form.Subscribe selector={(s) => [s.isSubmitting] as const}>
        {([isSubmitting]) => (
          <FormActions
            canSubmit={ready} isSubmitting={isSubmitting || busy}
            submitLabel={stored ? 'Save schedule' : 'Create schedule'} onCancel={onCancel}
            hint={ready
              ? `${cronToText(v.cron)} · ${recipients.toLocaleString('en')} ${recipients === 1 ? 'recipient' : 'recipients'}`
              : 'Choose a report and who receives it to continue.'}
          />
        )}
      </form.Subscribe>
    </form>
  )
}
