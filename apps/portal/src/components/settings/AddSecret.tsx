import { useState } from 'react'
import { TextInput } from '@mantine/core'
import { useQueryClient } from '@tanstack/react-query'
import { Field } from '../form/Field'
import { keyValueProblem } from '../../lib/maps'
import { afterSetting, brokenWithout, entryFor, secretNameProblem } from '../../lib/secrets'
import type { SecretEntry, SecretList } from '../../lib/types'
import { SecretValueForm } from './SecretValueForm'

/**
 * A secret nothing has asked for yet, or one somebody would rather name first.
 *
 * The name is checked here against the server's rule as it is typed, so the
 * button only works for a name the server will take — and whatever it refuses
 * anyway, a full project or a deployment with no key, comes back as its own
 * sentence under the value.
 *
 * A name the list already has is said out loud before saving, because adding
 * one that exists is replacing it, and the difference matters most when it is
 * a warehouse's password somebody else set.
 */
export function AddSecret({ list, onAdded }: {
  list: SecretList
  onAdded: (sentence: string) => void
}) {
  const [name, setName] = useState('')
  const [touched, setTouched] = useState(false)
  const queries = useQueryClient()

  const problem = secretNameProblem(name)
  // A character no name can hold is wrong at once; anything else — a dot at
  // the start, say — waits until the field is left, when the name is finished.
  const shown = name !== '' && (touched || /[^A-Za-z0-9_.-]/.test(name)) ? problem : undefined
  const existing = problem ? undefined : entryFor(list, name)

  return (
    // The band spans the card and the fields do not: a grey panel that stops
    // short of the card's edge reads as a second, broken card.
    <div className="border-b border-line bg-sunken p-4" data-testid="add-secret-form">
      <div className="grid max-w-2xl gap-4">
        <Field label="Name" error={shown}
          help={<>What a definition writes as <code className="font-mono">{'${secret:name}'}</code>: letters, digits, dots, dashes and underscores.</>}>
          <TextInput value={name} data-testid="secret-name" autoComplete="off" spellCheck={false}
            placeholder="warehouse_password" classNames={{ input: 'font-mono' }}
            onBlur={() => setTouched(true)}
            onChange={(e) => setName(e.currentTarget.value)} />
        </Field>

        {existing && (
          <p className="text-small text-ink-secondary" data-testid="secret-exists">{already(existing)}</p>
        )}

        <SecretValueForm name={name} label="Value" submitLabel="Add" ready={!problem}
          help="Stored sealed, and never shown again — not here, not to anybody."
          check={(v) => keyValueProblem(name, v)}
          onSaved={() => onAdded(afterSetting(name,
            entryFor(queries.getQueryData<SecretList>(['secrets']), name)))} />
      </div>
    </div>
  )
}

/** What adding a name the list already has will do. */
function already(entry: SecretEntry): string {
  switch (entry.source) {
    case 'project':
      return 'A value is already stored under this name. Adding one replaces it.'
    case 'deployment':
      return 'The server’s environment answers this name today. A value stored here takes its '
        + 'place, for this project only.'
    default:
      return entry.usedBy.length > 0
        ? `Nothing answers this name yet: ${brokenWithout(entry.usedBy)}. Storing it fixes that.`
        : 'Nothing answers this name yet.'
  }
}
