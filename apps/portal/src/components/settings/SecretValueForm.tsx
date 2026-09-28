import { useState, type ReactNode } from 'react'
import { Button, PasswordInput } from '@mantine/core'
import { useQueryClient } from '@tanstack/react-query'
import { Field } from '../form/Field'
import { ApiError, setSecret } from '../../lib/api'
import { secretValueProblem } from '../../lib/secrets'

interface Props {
  /** The secret this stores, already decided — this form never asks for a name. */
  name: string
  label: string
  help?: ReactNode
  submitLabel?: string
  /** A rule of the caller's own on top of the server's, such as Mapbox's pk. */
  check?: (value: string) => string | undefined
  /** False while something else the save needs — a name being typed — is not right yet. */
  ready?: boolean
  onSaved: () => void
  onCancel?: () => void
}

/**
 * One value, sent once, never shown again.
 *
 * Starts empty every time, and not as a courtesy: there is no endpoint that
 * returns a value, so there is nothing it could be filled with — and a field
 * that looked filled would be a promise that one exists. The value leaves
 * React state as soon as the server has it.
 *
 * Not a <form>. It is opened inside the report builder, which is one, and a
 * form inside a form is not HTML; Enter is caught here instead, so pressing it
 * saves the key rather than submitting the report around it.
 */
export function SecretValueForm({
  name, label, help, submitLabel = 'Save', check, ready = true, onSaved, onCancel,
}: Props) {
  const [value, setValue] = useState('')
  const [problem, setProblem] = useState('')
  const [busy, setBusy] = useState(false)
  const queries = useQueryClient()

  async function save() {
    if (!ready) return
    const local = secretValueProblem(value) ?? check?.(value)
    if (local) {
      setProblem(local)
      return
    }
    setBusy(true)
    setProblem('')
    try {
      await setSecret(name, value)
      setValue('')
      // Awaited, so the row this sits in already says Stored when it closes.
      await queries.invalidateQueries({ queryKey: ['secrets'] })
      onSaved()
    } catch (err) {
      // The server's sentence, whole: it is the one that knows whether this was
      // a name it refuses, a deployment with no key, or a project that is full.
      setProblem(err instanceof ApiError ? err.message : 'Could not reach the server.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid gap-3" data-testid="secret-form">
      <Field label={label} help={help}>
        {/*
          new-password, so a browser does not fill in the password somebody
          signs in with — which would then be stored as a warehouse's, or sent
          to every reader as a map key. The two ignore attributes ask the same
          of the password managers that do not read autocomplete.
        */}
        <PasswordInput value={value} autoComplete="new-password" data-1p-ignore data-lpignore="true"
          data-testid="secret-value" aria-label={`Value for ${name}`}
          onChange={(e) => setValue(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key !== 'Enter') return
            e.preventDefault()
            void save()
          }} />
      </Field>
      {problem && (
        <p role="alert" data-testid="secret-error" className="text-small text-ink">{problem}</p>
      )}
      <div className="flex gap-2">
        <Button type="button" size="xs" loading={busy} disabled={value === '' || !ready}
          data-testid="secret-save" onClick={() => void save()}>
          {submitLabel}
        </Button>
        {onCancel && (
          <Button type="button" size="xs" variant="default" disabled={busy} onClick={onCancel}>
            Cancel
          </Button>
        )}
      </div>
    </div>
  )
}
