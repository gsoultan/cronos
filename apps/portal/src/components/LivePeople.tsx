import { useState } from 'react'
import { Button, PasswordInput, Select, TextInput } from '@mantine/core'
import { Field } from './form/Field'
import { EmptyState } from './EmptyState'
import { relativeTime } from '../lib/format'
import { usePeople } from '../lib/usePeople'
import { addPerson, amendPerson, ApiError, type Person } from '../lib/api'

const CARD = 'mb-6 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'
const ROW = 'flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0'

const ROLES = [
  { value: 'admin', label: 'Admin — manages people and settings' },
  { value: 'editor', label: 'Editor — changes definitions' },
  { value: 'viewer', label: 'Viewer — reads reports' },
]

/**
 * Who has access, as the server has it.
 *
 * One organisation, one project and one role each — which is what the server
 * models, and less than the sample directory beside it shows. The sample one
 * describes several projects and a person in some of them; there has never
 * been a membership table behind it. Showing the smaller true thing when
 * connected is better than showing the larger one and having half of it fail.
 *
 * Until this existed, taking somebody's access away meant a SQL statement
 * against production.
 */
export function LivePeople() {
  const { data, isPending, error, refresh } = usePeople()
  const [adding, setAdding] = useState(false)
  const [refused, setRefused] = useState('')

  if (isPending) {
    return <p data-testid="people-loading" className="p-8 text-center text-ink-muted">Loading…</p>
  }
  if (error) {
    // The endpoint refuses everybody but an administrator, list included, so
    // this is the ordinary answer for an editor rather than a fault.
    return (
      <EmptyState title="Only an administrator can see who has access"
        description="The list of who works here, what their addresses are and when each of them last signed in is a description of your organisation. Ask an administrator if you need somebody added." />
    )
  }

  const people = data?.people ?? []

  return (
    <>
      <section className={CARD} data-testid="live-people">
        <div className={HEAD}>
          <div>
            <h2 className="text-lead font-semibold text-ink">People with access</h2>
            <p className="mt-1 text-small text-ink-secondary">
              Everyone here reaches this project. Turning access off keeps the account,
              so a run they started is still attributable to somebody.
            </p>
          </div>
          <Button onClick={() => setAdding((v) => !v)} data-testid="add-person">
            {adding ? 'Cancel' : 'Add someone'}
          </Button>
        </div>

        {adding && (
          <AddPerson
            onAdded={() => { setAdding(false); void refresh() }}
            onRefused={setRefused} />
        )}

        {refused && (
          <p role="alert" data-testid="people-error"
            className="border-b border-line bg-serious/10 px-4 py-2 text-small text-ink">
            {refused}
          </p>
        )}

        <ul>
          {people.map((person) => (
            <PersonRow key={person.id} person={person}
              onChanged={() => void refresh()} onRefused={setRefused} />
          ))}
        </ul>
      </section>
    </>
  )
}

function PersonRow({ person, onChanged, onRefused }: {
  person: Person
  onChanged: () => void
  onRefused: (message: string) => void
}) {
  const [busy, setBusy] = useState(false)

  async function amend(change: { role?: string; disabled?: boolean }) {
    setBusy(true)
    onRefused('')
    try {
      await amendPerson(person.id, change)
      onChanged()
    } catch (err) {
      onRefused(err instanceof ApiError ? err.message : 'Could not reach the server.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <li className={`${ROW} ${person.disabled ? 'opacity-60' : ''}`} data-testid="person">
      <span className="grid min-w-[220px] flex-1 gap-0.5">
        <span className="font-semibold text-ink">{person.name || person.email}</span>
        <span className="text-small text-ink-secondary">{person.email}</span>
      </span>

      {/* Said plainly rather than shown as a greyed row alone: "disabled" is
          the difference between somebody who works here and somebody who does
          not, and it should not be a shade. */}
      {person.disabled && (
        <span data-testid="person-disabled"
          className="rounded-full bg-serious/20 px-2 py-px text-micro font-medium text-ink">
          access off
        </span>
      )}

      <span className="text-caption text-ink-muted">
        {person.lastSeen ? `last seen ${relativeTime(person.lastSeen)}` : 'never signed in'}
      </span>

      <Select data={ROLES} value={person.role} allowDeselect={false} w={190} size="xs"
        disabled={busy || person.disabled} aria-label={`Role for ${person.email}`}
        onChange={(v) => v && void amend({ role: v })} />

      <button type="button" disabled={busy} data-testid="toggle-access"
        onClick={() => void amend({ disabled: !person.disabled })}
        className="shrink-0 cursor-pointer text-small text-ink-muted underline hover:text-ink">
        {person.disabled ? 'Restore access' : 'Turn access off'}
      </button>
    </li>
  )
}

/**
 * Adding somebody.
 *
 * A password to hand over rather than an invitation to send. It is what the
 * server does, and the form says so instead of implying an email is on its
 * way — a person waiting for one that does not exist is worse than being
 * told to send a message themselves.
 */
function AddPerson({ onAdded, onRefused }: {
  onAdded: () => void
  onRefused: (message: string) => void
}) {
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [role, setRole] = useState('viewer')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)

  const ready = email.includes('@') && password.trim().length >= 12

  async function submit() {
    setBusy(true)
    onRefused('')
    try {
      await addPerson({ email, name, role, password })
      onAdded()
    } catch (err) {
      onRefused(err instanceof ApiError ? err.message : 'Could not reach the server.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid gap-4 border-b border-line bg-sunken p-4 md:grid-cols-2">
      <Field label="Email">
        <TextInput value={email} type="email" data-testid="person-email"
          onChange={(e) => setEmail(e.currentTarget.value)} />
      </Field>
      <Field label="Name" required={false}>
        <TextInput value={name} onChange={(e) => setName(e.currentTarget.value)} />
      </Field>
      <Field label="Role">
        <Select data={ROLES} value={role} allowDeselect={false}
          onChange={(v) => setRole(v ?? 'viewer')} />
      </Field>
      <Field label="First password"
        help="Give it to them directly. They change it themselves from Account.">
        <PasswordInput value={password} data-testid="person-password"
          onChange={(e) => setPassword(e.currentTarget.value)} />
      </Field>
      <div className="md:col-span-2">
        <Button onClick={submit} loading={busy} disabled={!ready} data-testid="save-person">
          Add them
        </Button>
      </div>
    </div>
  )
}
