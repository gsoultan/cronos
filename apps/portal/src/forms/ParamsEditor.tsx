import { Checkbox, Select, TagsInput, TextInput } from '@mantine/core'
import { Field } from '../components/form/Field'
import type { Param } from '../lib/types'

const TYPES = [
  { value: 'string', label: 'Text' },
  { value: 'number', label: 'Number' },
  { value: 'date', label: 'Date' },
  { value: 'bool', label: 'True or false' },
  { value: 'enum', label: 'One of a list' },
]

/**
 * The questions a dataset accepts.
 *
 * The only caller-supplied input that reaches a query, and it reaches it as a
 * bind argument — which is why this editor asks for a type and not for a
 * fragment of SQL. A report that needs a different shape of query is a
 * different dataset, not a cleverer parameter.
 *
 * Editable here because they were not: a parameterised dataset survived being
 * opened and saved, and could only be created by editing the file. That made
 * the portal a second-class way to author exactly the datasets that need the
 * most care.
 */
export function ParamsEditor({ params, onChange }: {
  params: Param[]
  onChange: (params: Param[]) => void
}) {
  const amend = (at: number, change: Partial<Param>) =>
    onChange(params.map((p, i) => (i === at ? { ...p, ...change } : p)))

  return (
    <div className="grid gap-4" data-testid="params-editor">
      {params.length === 0 && (
        <p className="text-small text-ink-secondary">
          None. A dataset with no parameters answers the same question every time,
          which is what most of them should do — add one when a report needs to ask.
        </p>
      )}

      {params.map((param, i) => (
        <div key={i} className="grid gap-3 rounded-lg border border-line bg-sunken p-4"
          data-testid="param">
          <div className="grid gap-3 md:grid-cols-2">
            <Field label="Name"
              help="What the query calls it: {{ .params.name }}.">
              <TextInput value={param.name} data-testid="param-name"
                classNames={{ input: 'font-mono' }}
                onChange={(e) => amend(i, { name: e.currentTarget.value })} />
            </Field>
            <Field label="Type"
              help="What turns a caller's JSON into something safe to bind.">
              <Select data={TYPES} value={param.type} allowDeselect={false}
                onChange={(v) => amend(i, { type: (v ?? 'string') as Param['type'] })} />
            </Field>
          </div>

          <div className="grid gap-3 md:grid-cols-2">
            <Field label="Label" required={false}
              help="What a reader sees above the control.">
              <TextInput value={param.label ?? ''}
                onChange={(e) => amend(i, { label: e.currentTarget.value })} />
            </Field>
            <Field label="Default" required={false}
              help="Omitted by a caller, this is used. Without one, required means required.">
              <TextInput value={param.default ?? ''} data-testid="param-default"
                onChange={(e) => amend(i, { default: e.currentTarget.value })} />
            </Field>
          </div>

          {/* Enum only, and required there: a list of permitted values on a
              type the engine does not check reads as a constraint and is not
              one. */}
          {param.type === 'enum' && (
            <Field label="Permitted values"
              help="An enum with no values accepts everything, which is the opposite of asking for one.">
              <TagsInput value={param.values ?? []} data-testid="param-values"
                placeholder="Add a value and press Enter"
                onChange={(v) => amend(i, { values: v })} />
            </Field>
          )}

          <div className="flex flex-wrap items-center gap-5">
            <Checkbox label="Required" checked={param.required ?? false}
              data-testid="param-required"
              onChange={(e) => amend(i, { required: e.currentTarget.checked })} />
            <Checkbox label="Accepts several" checked={param.multiple ?? false}
              description="Bound as a list, for IN and = ANY(…)."
              onChange={(e) => amend(i, { multiple: e.currentTarget.checked })} />

            <button type="button" data-testid="remove-param"
              onClick={() => onChange(params.filter((_, x) => x !== i))}
              className="ml-auto cursor-pointer text-small text-ink-muted underline hover:text-ink">
              Remove
            </button>
          </div>
        </div>
      ))}

      <button type="button" data-testid="add-param"
        onClick={() => onChange([...params, { name: '', type: 'string' }])}
        className="cursor-pointer justify-self-start text-small text-ink-muted underline hover:text-ink">
        Add a parameter
      </button>
    </div>
  )
}
