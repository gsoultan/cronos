import { useState } from 'react'
import { Button, Select } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  changeGrant, connected, listGroups, listPeople, reportGrants,
  type GrantKind, type Person,
} from '../../lib/api'
import { Tag } from '../StatusPill'

/**
 * Who may open one report.
 *
 * Shown against the report rather than in a grants collection, because that is
 * the question an administrator arrives with: not "what grants exist" but "who
 * can see this".
 *
 * The distinction this panel exists to make legible is between *open* and
 * *granted to nobody*. Both are an empty list, they are opposite states, and
 * the server sends `restricted` so the difference is read rather than guessed —
 * a padlock drawn from `grants.length` would say "restricted" about a report
 * everyone can see the moment the last grant is removed.
 */
export function AccessPanel({ report, canAdmin }: { report: string; canAdmin: boolean }) {
  const client = useQueryClient()
  const [kind, setKind] = useState<GrantKind>('group')
  const [subject, setSubject] = useState('')

  const access = useQuery({
    queryKey: ['grants', report],
    queryFn: () => reportGrants(report),
    enabled: connected() && canAdmin,
  })
  // Offered as a list, because a grant to a group that does not exist is a
  // permission that silently opens nothing.
  const groups = useQuery({
    queryKey: ['groups'],
    queryFn: listGroups,
    enabled: connected() && canAdmin,
  })
  // And the roster, for the same reason: a grant typed as an account id is a
  // permission nobody can read back, and one typo makes it a permission
  // nobody holds.
  const roster = useQuery({
    queryKey: ['people'],
    queryFn: listPeople,
    enabled: connected() && canAdmin,
  })

  const change = useMutation({
    mutationFn: ({ k, s, give }: { k: GrantKind; s: string; give: boolean }) =>
      changeGrant(report, k, s, give),
    onSuccess: () => {
      setSubject('')
      void client.invalidateQueries({ queryKey: ['grants', report] })
      // The catalogue hides what the caller may not open, so restricting a
      // report changes the list this panel was opened from.
      void client.invalidateQueries({ queryKey: ['catalog'] })
    },
  })

  if (!connected() || !canAdmin) return null

  const grants = access.data?.grants ?? []
  const restricted = access.data?.restricted ?? false

  const people = roster.data?.people ?? []
  const named = (g: { kind: GrantKind; subject: string }) => {
    if (g.kind !== 'user') return g.subject
    const p = people.find((x) => x.id === g.subject)
    return p ? personLabel(p) : g.subject
  }
  // Somebody who already holds a grant is not worth offering: the server is
  // idempotent, so granting again succeeds and changes nothing, which reads as
  // a button that did not work.
  const already = new Set(grants.filter((g) => g.kind === 'user').map((g) => g.subject))
  const ungranted = people.filter((p) => !already.has(p.id) && !p.disabled)

  return (
    <section className="rounded-lg border border-line bg-surface p-4" data-testid="access-panel">
      <h3 className="text-body font-semibold text-ink">Who can open this</h3>

      <p className="mt-1 max-w-[62ch] text-small text-ink-secondary">
        {restricted
          ? 'Only the people and groups below, plus project administrators. ' +
            'Everybody else does not see this report at all.'
          : 'Everybody in this project. Grant it to somebody and it becomes ' +
            'visible only to them.'}
      </p>

      {access.isLoading && (
        <p className="mt-3 text-small text-ink-secondary">Reading…</p>
      )}

      {grants.length > 0 && (
        <ul className="mt-3 flex flex-wrap gap-2">
          {grants.map((g) => (
            <li key={`${g.kind}:${g.subject}`}>
              <span className="inline-flex items-center gap-1.5 rounded-full border border-line px-2.5 py-1 text-small">
                <Tag>{g.kind}</Tag>
                <span className="text-ink">{named(g)}</span>
                <button type="button" aria-label={`Remove ${g.subject}`}
                  className="cursor-pointer text-ink-secondary hover:text-bad"
                  onClick={() => change.mutate({ k: g.kind, s: g.subject, give: false })}>
                  ×
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}

      <div className="mt-3 flex flex-wrap items-end gap-2">
        <Select size="xs" label="Grant to" w={110} allowDeselect={false}
          data={[{ value: 'group', label: 'Group' }, { value: 'user', label: 'Person' }]}
          value={kind} onChange={(v) => { setKind((v ?? 'group') as GrantKind); setSubject('') }} />

        {kind === 'group' ? (
          <Select size="xs" label="Group" w={220} searchable
            placeholder={groups.data?.groups.length ? 'Choose a group' : 'No groups yet'}
            disabled={!groups.data?.groups.length}
            nothingFoundMessage="No group by that name"
            data={(groups.data?.groups ?? []).map((g) => ({ value: g.name, label: g.name }))}
            value={subject || null} onChange={(v) => setSubject(v ?? '')} />
        ) : (
          <Select size="xs" label="Person" w={260} searchable
            placeholder={ungranted.length ? 'Choose somebody' : 'Everybody is already named'}
            disabled={!ungranted.length}
            nothingFoundMessage="Nobody by that name"
            data={ungranted.map((p) => ({ value: p.id, label: personLabel(p) }))}
            value={subject || null} onChange={(v) => setSubject(v ?? '')} />
        )}

        <Button size="xs" disabled={!subject} loading={change.isPending}
          onClick={() => change.mutate({ k: kind, s: subject, give: true })}>
          Grant
        </Button>
      </div>

      {change.isError && (
        <p className="mt-2 text-small text-bad">{(change.error as Error).message}</p>
      )}

      {!restricted && grants.length === 0 && (
        <p className="mt-3 text-micro text-ink-secondary">
          A report stays open until its first grant. Administrators keep access
          either way — otherwise a mistake here would need a database to undo.
        </p>
      )}
    </section>
  )
}

/**
 * Somebody's name, falling back to what we have.
 *
 * A grant reads as a permission somebody gave to a person, so it should name
 * one. "usr_42b2ab168bade063" is the key the server stores it under and cannot
 * be checked against the person who was meant.
 */
function personLabel(p: Person): string {
  return p.name ? `${p.name} (${p.email})` : p.email
}
