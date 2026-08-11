import { useState } from 'react'
import { Button, Select } from '@mantine/core'
import { RoleSelect } from './RoleSelect'
import { Tag } from '../StatusPill'
import { relativeTime } from '../../lib/format'
import {
  isLastOwner, ORG_ROLES, PROJECT_ROLES, projectsFor, reachesAllProjects,
  type Person,
} from '../../lib/people'
import { projects as allProjects } from '../../lib/workspace'

interface Props {
  person: Person
  everyone: Person[]
  canAdmin: boolean
  onChangeOrgRole: (role: string) => void
  onChangeProjectRole: (projectId: string, role: string | null) => void
  onRemove: () => void
}

/**
 * A last-seen time can only be in the past. Clock skew between a database and a
 * browser is ordinary, and rendering it as "in 54 minutes" makes the product
 * look broken over a discrepancy that means nothing.
 */
function lastSeen(at?: string): string {
  if (!at) return 'Never signed in'
  return new Date(at).getTime() > Date.now() ? 'Just now' : relativeTime(at)
}

export function PersonRow({
  person, everyone, canAdmin, onChangeOrgRole, onChangeProjectRole, onRemove,
}: Props) {
  const [open, setOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)

  const lastOwner = isLastOwner(person, everyone)
  const viaOrg = reachesAllProjects(person)
  const reach = projectsFor(person)

  /* Two rules, both stated where the choice is made rather than after it. */
  const lockedReason = lastOwner
    ? 'An organization needs at least one owner'
    : person.isYou ? 'You cannot change your own role' : undefined

  return (
    <li className="border-b border-line last:border-b-0">
      <div className="flex flex-wrap items-center gap-4 px-4 py-3">
        <div className="grid min-w-[200px] flex-1 gap-px">
          <span className="flex items-center gap-2 font-medium text-ink">
            {person.name}
            {person.isYou && <Tag>you</Tag>}
          </span>
          <span className="text-caption text-ink-muted">{person.email}</span>
        </div>

        <button type="button" onClick={() => setOpen((o) => !o)}
          className="min-w-[180px] cursor-pointer text-left text-small text-ink-secondary
                     hover:text-ink"
          aria-expanded={open}>
          {viaOrg
            ? <>All projects <span className="text-ink-muted">· via {person.orgRole}</span></>
            : reach.length === 0
              ? <span className="text-ink-muted">No projects yet</span>
              : <>{reach.length} project{reach.length === 1 ? '' : 's'}{' '}
                  <span className="text-ink-muted">
                    · {reach.map((r) => r.project.name).join(', ')}
                  </span></>}
        </button>

        <span className="w-[110px] text-caption text-ink-muted">
          {lastSeen(person.lastActive)}
        </span>

        {canAdmin ? (
          <RoleSelect value={person.orgRole} options={ORG_ROLES} label={`Role for ${person.name}`}
            onChange={onChangeOrgRole} lockedReason={lockedReason} />
        ) : (
          <Tag>{person.orgRole}</Tag>
        )}

        {canAdmin && (
          <Button variant="subtle" color="gray" size="xs"
            disabled={person.isYou || lastOwner}
            title={person.isYou ? 'You cannot remove yourself'
              : lastOwner ? 'An organization needs at least one owner' : undefined}
            onClick={() => setConfirming(true)}>
            Remove
          </Button>
        )}
      </div>

      {/* Destructive actions confirm in place. A modal would hide the row you
          are deciding about, which is the thing you want to look at. */}
      {confirming && (
        <div className="flex flex-wrap items-center gap-3 border-t border-line bg-sunken px-4 py-3">
          <p className="flex-1 text-small text-ink-secondary">
            Remove <strong className="text-ink">{person.name}</strong>? They lose access to{' '}
            {viaOrg ? 'every project in this organization' :
              reach.length === 0 ? 'nothing — they have no projects yet' :
              `${reach.length} project${reach.length === 1 ? '' : 's'}`}
            . Reports they built stay.
          </p>
          <Button variant="default" size="xs" onClick={() => setConfirming(false)}>Cancel</Button>
          <Button size="xs" color="red" onClick={() => { setConfirming(false); onRemove() }}>
            Remove
          </Button>
        </div>
      )}

      {open && (
        <div className="border-t border-line bg-sunken px-4 py-3">
          {viaOrg ? (
            <p className="text-small text-ink-secondary">
              Organization {person.orgRole}s reach every project without being added to
              them. Change their role to Member to grant projects individually.
            </p>
          ) : (
            <ul className="grid gap-2">
              {allProjects.filter((p) => p.orgId === person.orgId).map((p) => {
                const current = person.projectRoles[p.id] ?? ''
                return (
                  <li key={p.id} className="flex items-center gap-3">
                    <span className="min-w-[160px] text-small text-ink">{p.name}</span>
                    {canAdmin ? (
                      <Select size="xs" w={190} value={current || 'none'} allowDeselect={false}
                        aria-label={`${person.name} in ${p.name}`}
                        data={[
                          { value: 'none', label: 'No access' },
                          ...PROJECT_ROLES.map((r) => ({ value: r.value, label: r.label })),
                        ]}
                        onChange={(v) => onChangeProjectRole(p.id, v === 'none' ? null : v)} />
                    ) : (
                      <Tag>{current || 'no access'}</Tag>
                    )}
                  </li>
                )
              })}
            </ul>
          )}
        </div>
      )}
    </li>
  )
}
