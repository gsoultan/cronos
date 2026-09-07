import { useState } from 'react'
import { Button, Select, TextInput } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { changeGrant, connected, listGroups, reportGrants, type GrantKind } from '../../lib/api'
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
                <span className="text-ink">{g.subject}</span>
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
          <Select size="xs" label="Group" w={200} searchable
            placeholder={groups.data?.groups.length ? 'Choose a group' : 'No groups yet'}
            disabled={!groups.data?.groups.length}
            data={(groups.data?.groups ?? []).map((g) => ({ value: g.name, label: g.name }))}
            value={subject || null} onChange={(v) => setSubject(v ?? '')} />
        ) : (
          <TextInput size="xs" label="Account id" w={240} placeholder="usr_…"
            value={subject} onChange={(e) => setSubject(e.currentTarget.value)} />
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
