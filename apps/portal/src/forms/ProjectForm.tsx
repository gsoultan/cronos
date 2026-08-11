import { useState } from 'react'
import { useForm, useStore } from '@tanstack/react-form'
import { TextInput, Textarea } from '@mantine/core'
import { Field, fieldError } from '../components/form/Field'
import { IdentifierField } from '../components/form/IdentifierField'
import { FormActions, FormSection, Callout } from '../components/form/FormShell'
import { all, maxLength, required, slug, toSlug } from '../lib/validators'
import { useWorkspace } from '../lib/WorkspaceContext'

interface Props {
  onDone: () => void
  onCancel: () => void
}

export function ProjectForm({ onDone, onCancel }: Props) {
  /* Once someone edits the API name, typing in Name must stop
     overwriting it — silently discarding a deliberate edit. */
  const [slugEdited, setSlugEdited] = useState(false)
  const { org } = useWorkspace()

  const form = useForm({
    defaultValues: { name: '', slug: '', description: '' },
    onSubmit: async () => { onDone() },
  })

  const values = useStore(form.store, (s) => s.values)
  const ready = values.name.trim().length > 1

  return (
    <form onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); form.handleSubmit() }}>
      <FormSection title={`New project in ${org.name}`}
        description="A project is an isolation boundary: its data sources, datasets and reports are its own, and cannot be reached from another project.">
        <form.Field name="name"
          validators={{ onBlur: ({ value }) => all(required('A name'), maxLength(60, 'The name'))(value) }}>
          {(f) => (
            <Field label="Name" error={fieldError(f.state.meta)}>
              <TextInput value={f.state.value} onBlur={f.handleBlur} placeholder="Finance"
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
                      usedFor="Every API path for this project contains it."
                      onChange={(v) => { setSlugEdited(true); f.handleChange(v) }} />
          )}
        </form.Field>

        <form.Field name="description">
          {(f) => (
            <Field label="Description" required={false}
              help="Helps people picking between projects in the switcher.">
              <Textarea autosize minRows={2} value={f.state.value} onBlur={f.handleBlur}
                onChange={(e) => f.handleChange(e.currentTarget.value)} />
            </Field>
          )}
        </form.Field>
      </FormSection>

      <Callout>
        You will be its first admin. Nobody else can see this project until you add them.
      </Callout>

      <form.Subscribe selector={(s) => [s.isSubmitting] as const}>
        {([isSubmitting]) => (
          <FormActions canSubmit={ready} isSubmitting={isSubmitting}
            submitLabel="Create project" onCancel={onCancel} />
        )}
      </form.Subscribe>
    </form>
  )
}
