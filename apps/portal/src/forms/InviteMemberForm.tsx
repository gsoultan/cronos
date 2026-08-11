import { useState } from 'react'
import { useForm, useStore } from '@tanstack/react-form'
import { TextInput } from '@mantine/core'
import { Field, fieldError } from '../components/form/Field'
import { FormActions, FormSection } from '../components/form/FormShell'
import { all, email, required } from '../lib/validators'

interface Props {
  scope: 'organization' | 'project'
  scopeName: string
  onDone: () => void
  onCancel: () => void
}

const ORG_ROLES = [
  { id: 'member', label: 'Member', hint: 'Belongs to the organization. Sees only the projects they are added to.' },
  { id: 'admin', label: 'Admin', hint: 'Manages members and projects, and can enter any project here.' },
  { id: 'owner', label: 'Owner', hint: 'Everything an admin can do, plus billing and deleting the organization.' },
]

const PROJECT_ROLES = [
  { id: 'viewer', label: 'Viewer', hint: 'Runs and views reports. Cannot change them.' },
  { id: 'editor', label: 'Editor', hint: 'Creates and edits datasets, reports and schedules.' },
  { id: 'admin', label: 'Admin', hint: 'Everything an editor can do, plus members, data sources and settings.' },
]

/**
 * Roles are radio cards with their consequence spelled out, not a dropdown of
 * words. "Editor" means nothing until someone tells you what an editor can do,
 * and the moment to say so is while the choice is being made.
 */
export function InviteMemberForm({ scope, scopeName, onDone, onCancel }: Props) {
  const roles = scope === 'organization' ? ORG_ROLES : PROJECT_ROLES
  const [role, setRole] = useState(roles[0]!.id)

  const form = useForm({
    defaultValues: { email: '' },
    onSubmit: async () => { onDone() },
  })

  const values = useStore(form.store, (s) => s.values)
  const ready = values.email.trim() !== '' && !email(values.email)

  return (
    <form onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); form.handleSubmit() }}>
      <FormSection title={`Add someone to ${scopeName}`}
        description={scope === 'organization'
          ? 'They will be able to sign in, but will not see any project until they are added to one.'
          : 'They must already be a member of this organization.'}>
        <form.Field name="email"
          validators={{ onBlur: ({ value }) => all(required('An email address'), email)(value) }}>
          {(f) => (
            <Field label="Email address" error={fieldError(f.state.meta)}>
              <TextInput type="email" value={f.state.value} onBlur={f.handleBlur}
                placeholder="dewi@acme.com"
                onChange={(e) => f.handleChange(e.currentTarget.value)} />
            </Field>
          )}
        </form.Field>

        <Field label="What can they do?">
          <div className="grid gap-2" role="radiogroup" aria-label="Role">
            {roles.map((r) => (
              <button key={r.id} type="button" role="radio" aria-checked={role === r.id}
                onClick={() => setRole(r.id)}
                className={`flex cursor-pointer items-start gap-3 rounded-md border bg-surface
                  px-4 py-3 text-left text-ink hover:border-accent
                  ${role === r.id ? 'border-accent bg-accent-wash' : 'border-line'}`}>
                <span aria-hidden
                  className={`mt-[3px] size-3.5 shrink-0 rounded-full border-2 ${
                    role === r.id
                      ? 'border-accent bg-accent shadow-[inset_0_0_0_3px_var(--color-surface)]'
                      : 'border-baseline bg-surface'}`} />
                <span>
                  <span className="block text-small font-semibold">{r.label}</span>
                  <span className="block text-small text-ink-secondary">{r.hint}</span>
                </span>
              </button>
            ))}
          </div>
        </Field>
      </FormSection>

      <form.Subscribe selector={(s) => [s.isSubmitting] as const}>
        {([isSubmitting]) => (
          <FormActions canSubmit={ready} isSubmitting={isSubmitting}
            submitLabel="Send invitation" onCancel={onCancel}
            hint="They get an email with a link. Nothing changes until they accept." />
        )}
      </form.Subscribe>
    </form>
  )
}
