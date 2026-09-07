import { useState } from 'react'
import { Button, Modal, Select, TextInput } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  changeGroupMember, createGroup, deleteGroup, groupMembers, listGroups,
  listPeople, setGroupScope, type Group, type Person,
} from '../../lib/api'
import { connected } from '../../lib/api'
import { Tag } from '../StatusPill'

const CARD = 'mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'

/**
 * Groups of people, and the rows each group reads through.
 *
 * Two jobs in one panel because they are two halves of one decision an
 * administrator makes at once: a group is who, and its scope is what they see.
 * Splitting them across two screens would mean creating "west" in one place and
 * remembering to confine it in another, and the failure of forgetting is a
 * group that reads everything.
 *
 * A scope is shown as its fields rather than as JSON. `region = west` is a
 * sentence somebody can check against what they meant; `{"region":"west"}` is a
 * format they have to parse first, and one they can break with a comma.
 */
export function GroupsPanel({ canAdmin }: { canAdmin: boolean }) {
  const client = useQueryClient()
  const [naming, setNaming] = useState(false)

  const groups = useQuery({
    queryKey: ['groups'],
    queryFn: listGroups,
    enabled: connected(),
  })

  const refresh = () => client.invalidateQueries({ queryKey: ['groups'] })

  if (!connected()) {
    return (
      <section className={CARD}>
        <div className="p-4 text-small text-ink-secondary">
          Groups live in the definition store, so this page needs a connected
          server. A file-backed deployment has nowhere to record one — nobody is
          restricted and nobody is confined.
        </div>
      </section>
    )
  }

  const list = groups.data?.groups ?? []

  return (
    <>
      <section className={CARD} data-testid="groups-panel">
        <div className={HEAD}>
          <div>
            <h2 className="text-lead font-semibold text-ink">Groups</h2>
            <p className="mt-1 max-w-[68ch] text-small text-ink-secondary">
              A group decides which reports its members can open, and — where you
              give it a scope — which rows they see inside them. A viewer with no
              scope reads everything the project reads, which is what they did
              before groups existed.
            </p>
          </div>
          {canAdmin && (
            <Button onClick={() => setNaming(true)} data-testid="new-group">
              New group
            </Button>
          )}
        </div>

        {groups.isLoading && (
          <p className="p-4 text-small text-ink-secondary">Reading the groups…</p>
        )}

        {!groups.isLoading && list.length === 0 && (
          <p className="p-4 text-small text-ink-secondary" data-testid="no-groups">
            No groups yet. Every report is open to everybody in the project until
            you grant one.
          </p>
        )}

        <ul>
          {list.map((g) => (
            <GroupRow key={g.id} group={g} canAdmin={canAdmin} onChange={refresh} />
          ))}
        </ul>
      </section>

      <NewGroup
        open={naming}
        onClose={() => setNaming(false)}
        onCreated={() => { setNaming(false); void refresh() }}
      />
    </>
  )
}

function GroupRow({ group, canAdmin, onChange }: {
  group: Group
  canAdmin: boolean
  onChange: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const scope = Object.entries(group.scope ?? {})

  const remove = useMutation({
    mutationFn: () => deleteGroup(group.id),
    onSuccess: onChange,
  })

  return (
    <li className="border-b border-line last:border-b-0">
      <div className="flex flex-wrap items-center justify-between gap-3 p-4">
        <div>
          <button type="button" onClick={() => setExpanded(!expanded)}
            className="cursor-pointer text-body font-medium text-ink hover:text-accent">
            {group.name}
          </button>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-small text-ink-secondary">
            <span>{group.members} {group.members === 1 ? 'person' : 'people'}</span>
            {scope.length === 0
              ? <Tag>reads everything</Tag>
              : scope.map(([field, value]) => (
                  <Tag key={field}>{field} = {value}</Tag>
                ))}
          </div>
        </div>
        {canAdmin && (
          <div className="flex gap-2">
            <Button size="xs" variant="default" onClick={() => setExpanded(!expanded)}>
              {expanded ? 'Done' : 'Edit'}
            </Button>
            <Button size="xs" variant="default" color="red"
              loading={remove.isPending}
              onClick={() => {
                // Said plainly, because deleting a group also withdraws every
                // report it opened — and somebody expecting to remove a label
                // would be removing an access decision.
                if (confirm(
                  `Delete "${group.name}"?\n\n` +
                  'Its members lose it, and every report granted to this group ' +
                  'stops opening for them.')) remove.mutate()
              }}>
              Delete
            </Button>
          </div>
        )}
      </div>

      {expanded && (
        <GroupDetail group={group} canAdmin={canAdmin} onChange={onChange} />
      )}
    </li>
  )
}

/**
 * Somebody's name, falling back to what we have.
 *
 * An account id is what the server keys membership on and the last thing an
 * administrator should have to read: "usr_42b2ab168bade063" and
 * "dewi@acme.example" identify the same person, and only one of them can be
 * checked against the person you meant.
 */
function label(p: Person): string {
  return p.name ? `${p.name} (${p.email})` : p.email
}

function GroupDetail({ group, canAdmin, onChange }: {
  group: Group
  canAdmin: boolean
  onChange: () => void
}) {
  const client = useQueryClient()
  const [account, setAccount] = useState('')
  const [field, setField] = useState(Object.keys(group.scope ?? {})[0] ?? '')
  const [value, setValue] = useState(Object.values(group.scope ?? {})[0] ?? '')

  const members = useQuery({
    queryKey: ['group-members', group.id],
    queryFn: () => groupMembers(group.id),
  })
  // The roster, so a member reads as a person rather than as the key the
  // server happens to store them under.
  const roster = useQuery({ queryKey: ['people'], queryFn: listPeople })
  const people = roster.data?.people ?? []
  const named = (id: string) => {
    const p = people.find((x) => x.id === id)
    return p ? label(p) : id
  }
  const refreshBoth = () => {
    void client.invalidateQueries({ queryKey: ['group-members', group.id] })
    onChange()
  }

  const member = useMutation({
    mutationFn: ({ user, join }: { user: string; join: boolean }) =>
      changeGroupMember(group.id, user, join),
    onSuccess: () => { setAccount(''); refreshBoth() },
  })
  const scope = useMutation({
    mutationFn: () => setGroupScope(group.id, field ? { [field]: value } : {}),
    onSuccess: refreshBoth,
  })

  /*
     Who can still be added: the roster, less whoever is already in, less
     anybody disabled.

     Offering somebody already in the group makes the Add button do nothing —
     the server is idempotent, so it succeeds and changes nothing, which reads
     as a bug. Offering a disabled account puts somebody in a group they cannot
     sign in to use, which reads as one later.
  */
  const inAlready = new Set(members.data?.members ?? [])
  const joinable = people.filter((p) => !inAlready.has(p.id) && !p.disabled)

  return (
    <div className="border-t border-line bg-surface-sunken p-4">
      <div className="grid gap-6 md:grid-cols-2">
        <div>
          <h3 className="mb-2 text-small font-semibold text-ink">Members</h3>
          {members.isLoading && <p className="text-small text-ink-secondary">Reading…</p>}
          <ul className="mb-3 space-y-1">
            {(members.data?.members ?? []).map((id) => (
              <li key={id} className="flex items-center justify-between gap-2 text-small">
                <span className="text-ink">{named(id)}</span>
                {canAdmin && (
                  <button type="button"
                    className="cursor-pointer text-micro text-ink-secondary hover:text-bad"
                    onClick={() => member.mutate({ user: id, join: false })}>
                    remove
                  </button>
                )}
              </li>
            ))}
            {members.data?.members.length === 0 && (
              <li className="text-small text-ink-secondary">Nobody yet.</li>
            )}
          </ul>
          {canAdmin && (
            <div className="flex gap-2">
              <Select size="xs" searchable className="grow"
                placeholder={joinable.length ? 'Add somebody…' : 'Everybody is already in'}
                disabled={!joinable.length}
                nothingFoundMessage="Nobody by that name"
                data={joinable.map((p) => ({ value: p.id, label: label(p) }))}
                value={account || null} onChange={(v) => setAccount(v ?? '')} />
              <Button size="xs" disabled={!account} loading={member.isPending}
                onClick={() => member.mutate({ user: account, join: true })}>
                Add
              </Button>
            </div>
          )}
        </div>

        <div>
          <h3 className="mb-2 text-small font-semibold text-ink">Rows these people see</h3>
          <p className="mb-2 max-w-[52ch] text-small text-ink-secondary">
            The field must be one the dataset's row-level security reads. Leave it
            blank and members read every row the project reads.
          </p>
          {canAdmin ? (
            <div className="flex flex-wrap items-end gap-2">
              <TextInput size="xs" label="Field" placeholder="region" value={field}
                onChange={(e) => setField(e.currentTarget.value)} w={130} />
              <TextInput size="xs" label="Value" placeholder="west" value={value}
                onChange={(e) => setValue(e.currentTarget.value)} w={130} />
              <Button size="xs" loading={scope.isPending} onClick={() => scope.mutate()}>
                Save
              </Button>
            </div>
          ) : (
            <p className="text-small text-ink-secondary">
              {Object.entries(group.scope ?? {}).map(([f, v]) => `${f} = ${v}`).join(', ')
                || 'Everything the project reads.'}
            </p>
          )}
          <p className="mt-2 text-micro text-ink-secondary">
            A change reaches people within a few seconds, on their next request.
          </p>
        </div>
      </div>
    </div>
  )
}

function NewGroup({ open, onClose, onCreated }: {
  open: boolean
  onClose: () => void
  onCreated: () => void
}) {
  const [name, setName] = useState('')
  const [field, setField] = useState('')
  const [value, setValue] = useState('')

  const create = useMutation({
    mutationFn: () => createGroup(name.trim(), field ? { [field]: value } : undefined),
    onSuccess: () => { setName(''); setField(''); setValue(''); onCreated() },
  })

  return (
    <Modal opened={open} onClose={onClose} title="New group" centered>
      <TextInput label="Name" placeholder="finance" value={name} mb={12}
        description="What a grant will refer to. One name per project."
        onChange={(e) => setName(e.currentTarget.value)} />

      <div className="flex gap-2">
        <TextInput label="Scope field" placeholder="region" value={field} className="grow"
          onChange={(e) => setField(e.currentTarget.value)} />
        <TextInput label="Value" placeholder="west" value={value} className="grow"
          onChange={(e) => setValue(e.currentTarget.value)} />
      </div>
      <p className="mt-2 text-micro text-ink-secondary">
        Optional. With a scope, members see only the rows that match it — and
        only where the dataset has row-level security on that field. Without
        one, the group decides which reports they open and nothing else.
      </p>

      {create.isError && (
        <p className="mt-3 text-small text-bad">
          {(create.error as Error).message}
        </p>
      )}

      <div className="mt-4 flex justify-end gap-2">
        <Button variant="default" onClick={onClose}>Cancel</Button>
        <Button disabled={!name.trim()} loading={create.isPending}
          onClick={() => create.mutate()}>Create</Button>
      </div>
    </Modal>
  )
}
