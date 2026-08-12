import { useEffect, useState } from 'react'
import { Button, Select, TextInput } from '@mantine/core'
import { Field } from './form/Field'
import { EmptyState } from './EmptyState'
import {
  amendAnywhere, ApiError, everyPerson, grantPlatform, type Person,
  platformAdmins, revokePlatform, type Tenant, tenants,
} from '../lib/api'

const CARD = 'mb-6 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'

const ROLES = [
  { value: 'admin', label: 'Admin' },
  { value: 'editor', label: 'Editor' },
  { value: 'viewer', label: 'Viewer' },
]

/**
 * Administering the deployment rather than a project.
 *
 * Everything else in Settings is scoped to the project somebody is in. This is
 * not, which is why it is only reachable by an account that administers the
 * deployment — and why the server answers 404 rather than 403 to anybody else,
 * so a portal that showed this to the wrong person would show it empty rather
 * than an error confirming the tier exists.
 *
 * What is deliberately absent is any way to open a project's reports. A platform
 * administrator moves people and grants permissions; reading a customer's data
 * still means joining their project, and the audit log records that it happened.
 */
export function LivePlatform({ me }: { me: string }) {
  const [reloads, setReloads] = useState(0)
  const [refused, setRefused] = useState('')

  return (
    <div key={reloads}>
      {refused && (
        <p role="alert" data-testid="platform-error"
          className="mb-4 rounded-lg border border-line bg-serious/10 px-4 py-2 text-small text-ink">
          {refused}
        </p>
      )}
      <Tenants onRefused={setRefused} />
      <Administrators me={me} onChanged={() => setReloads((n) => n + 1)} onRefused={setRefused} />
      <Everybody me={me} onChanged={() => setReloads((n) => n + 1)} onRefused={setRefused} />
    </div>
  )
}

/** Which organisations and projects exist, and how many people are in each. */
function Tenants({ onRefused }: { onRefused: (m: string) => void }) {
  const [rows, setRows] = useState<Tenant[] | null>(null)

  useEffect(() => {
    let live = true
    tenants()
      .then((out) => { if (live) setRows(out.tenants) })
      .catch((err: unknown) => {
        if (live) onRefused(err instanceof ApiError ? err.message : 'Could not read the tenants.')
      })
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <section className={CARD} data-testid="platform-tenants">
      <div className={HEAD}>
        <div>
          <h2 className="text-lead font-semibold text-ink">Tenants</h2>
          <p className="mt-1 text-small text-ink-secondary">
            Where accounts actually are, which is not the same question as what
            this process was configured to serve — a project with people and no
            server behind it is worth seeing rather than hiding.
          </p>
        </div>
      </div>
      {rows === null ? (
        <p className="p-8 text-center text-ink-muted">Loading…</p>
      ) : rows.length === 0 ? (
        <p className="p-8 text-center text-small text-ink-muted">No accounts anywhere yet.</p>
      ) : (
        <ul>
          {rows.map((t) => (
            <li key={`${t.org}/${t.project}`}
              className="flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0">
              <span className="min-w-[240px] flex-1 font-medium text-ink">
                {t.org}<span className="text-ink-muted"> / </span>{t.project}
              </span>
              <span className="text-small text-ink-secondary">
                {t.people} {t.people === 1 ? 'person' : 'people'}
              </span>
              {t.disabled > 0 && (
                <span className="text-caption text-ink-muted">{t.disabled} turned off</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

/** Who administers the deployment. */
function Administrators({ me, onChanged, onRefused }: {
  me: string
  onChanged: () => void
  onRefused: (m: string) => void
}) {
  const [rows, setRows] = useState<Person[] | null>(null)
  const [busy, setBusy] = useState('')

  useEffect(() => {
    let live = true
    platformAdmins()
      .then((out) => { if (live) setRows(out.admins) })
      .catch((err: unknown) => {
        if (live) onRefused(err instanceof ApiError ? err.message : 'Could not read the administrators.')
      })
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const admins = rows ?? []

  return (
    <section className={CARD} data-testid="platform-admins">
      <div className={HEAD}>
        <div>
          <h2 className="text-lead font-semibold text-ink">Deployment administrators</h2>
          <p className="mt-1 max-w-[68ch] text-small text-ink-secondary">
            They manage accounts, projects and this list. They cannot read any
            project&rsquo;s reports without being a member of it — which is what
            keeps one of these credentials from being every customer&rsquo;s data
            at once.
          </p>
        </div>
      </div>
      <ul>
        {admins.map((a) => (
          <li key={a.id}
            className="flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0">
            <span className="min-w-[240px] flex-1 font-medium text-ink">{a.email}</span>
            <span className="text-caption text-ink-muted">{a.org} / {a.project}</span>
            <Button variant="subtle" color="gray" size="xs" loading={busy === a.id}
              data-testid={`revoke-${a.id}`}
              onClick={() => {
                setBusy(a.id)
                onRefused('')
                revokePlatform(a.id)
                  .then(onChanged)
                  .catch((err: unknown) =>
                    onRefused(err instanceof ApiError ? err.message : 'Could not revoke it.'))
                  .finally(() => setBusy(''))
              }}>
              {/* Their own is not special-cased away: stepping down is a real
                  thing to want, and the server refuses only the last one. */}
              {a.id === me ? 'Step down' : 'Revoke'}
            </Button>
          </li>
        ))}
        {admins.length === 1 && (
          <li className="border-b border-line bg-sunken px-4 py-2 text-caption text-ink-muted last:border-b-0">
            This is the only one. Granting it to somebody else is what makes it
            possible to take away — a deployment with none cannot make another
            except from the command line.
          </li>
        )}
      </ul>
    </section>
  )
}

/** Every account, in every project. */
function Everybody({ me, onChanged, onRefused }: {
  me: string
  onChanged: () => void
  onRefused: (m: string) => void
}) {
  const [rows, setRows] = useState<Person[] | null>(null)
  const [query, setQuery] = useState('')

  useEffect(() => {
    let live = true
    everyPerson()
      .then((out) => { if (live) setRows(out.people) })
      .catch((err: unknown) => {
        if (live) onRefused(err instanceof ApiError ? err.message : 'Could not read the accounts.')
      })
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (rows === null) {
    return <p className="p-8 text-center text-ink-muted">Loading…</p>
  }
  const needle = query.trim().toLowerCase()
  const shown = needle
    ? rows.filter((p) => `${p.email} ${p.name ?? ''} ${p.org} ${p.project}`
        .toLowerCase().includes(needle))
    : rows

  if (rows.length === 0) {
    return <EmptyState title="No accounts" description="Nobody has an account on this deployment." />
  }

  return (
    <section className={CARD} data-testid="platform-people">
      <div className={HEAD}>
        <div>
          <h2 className="text-lead font-semibold text-ink">Every account</h2>
          <p className="mt-1 text-small text-ink-secondary">
            Across every organisation. Moving somebody between organisations is
            the one change no project administrator can make, which is why it is
            here.
          </p>
        </div>
        <TextInput size="xs" w={220} placeholder="Search accounts…" value={query}
          aria-label="Search accounts" data-testid="platform-search"
          onChange={(e) => setQuery(e.currentTarget.value)} />
      </div>
      <ul>
        {shown.map((p) => (
          <PersonAnywhere key={p.id} person={p} isMe={p.id === me}
            onChanged={onChanged} onRefused={onRefused} />
        ))}
      </ul>
    </section>
  )
}

function PersonAnywhere({ person, isMe, onChanged, onRefused }: {
  person: Person
  isMe: boolean
  onChanged: () => void
  onRefused: (m: string) => void
}) {
  const [moving, setMoving] = useState(false)
  const [org, setOrg] = useState(person.org)
  const [project, setProject] = useState(person.project)
  const [role, setRole] = useState<string>(person.role)
  const [busy, setBusy] = useState(false)

  function act(change: Parameters<typeof amendAnywhere>[1], after?: () => void) {
    setBusy(true)
    onRefused('')
    amendAnywhere(person.id, change)
      .then(() => { after?.(); onChanged() })
      .catch((err: unknown) =>
        onRefused(err instanceof ApiError ? err.message : 'Could not change that.'))
      .finally(() => setBusy(false))
  }

  return (
    <li className="border-b border-line px-4 py-3 last:border-b-0">
      <div className="flex flex-wrap items-center gap-4">
        <span className="min-w-[220px] flex-1">
          <span className="font-medium text-ink">{person.email}</span>
          {person.name && <span className="ml-2 text-small text-ink-secondary">{person.name}</span>}
        </span>
        <span className="text-caption text-ink-muted">
          {person.org} / {person.project} · {person.role}
        </span>
        {person.disabled && <span className="text-caption text-ink-muted">turned off</span>}

        <Button variant="subtle" color="gray" size="xs"
          onClick={() => setMoving((v) => !v)}>
          {moving ? 'Cancel' : 'Move'}
        </Button>
        {!isMe && (
          <Button variant="subtle" color="gray" size="xs" loading={busy}
            onClick={() => act({ disabled: !person.disabled })}>
            {person.disabled ? 'Turn on' : 'Turn off'}
          </Button>
        )}
        {!isMe && (
          <Button variant="subtle" color="gray" size="xs" loading={busy}
            data-testid={`grant-${person.id}`}
            onClick={() => {
              setBusy(true)
              onRefused('')
              grantPlatform(person.id)
                .then(onChanged)
                .catch((err: unknown) =>
                  onRefused(err instanceof ApiError ? err.message : 'Could not grant it.'))
                .finally(() => setBusy(false))
            }}>
            Make administrator
          </Button>
        )}
      </div>

      {moving && (
        <div className="mt-3 grid gap-3 rounded-lg bg-sunken p-3 sm:grid-cols-4">
          <Field label="Organisation">
            <TextInput size="xs" value={org} onChange={(e) => setOrg(e.currentTarget.value)} />
          </Field>
          <Field label="Project">
            <TextInput size="xs" value={project}
              onChange={(e) => setProject(e.currentTarget.value)} />
          </Field>
          <Field label="Role">
            <Select size="xs" data={ROLES} value={role} allowDeselect={false}
              onChange={(v) => setRole(v ?? 'viewer')} />
          </Field>
          <div className="flex items-end">
            <Button size="xs" loading={busy}
              disabled={org.trim() === '' || project.trim() === ''}
              onClick={() => act({ org, project, role }, () => setMoving(false))}>
              Move them
            </Button>
          </div>
        </div>
      )}
    </li>
  )
}
