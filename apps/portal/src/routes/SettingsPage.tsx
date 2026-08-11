import { useMemo, useState } from 'react'
import { Button, TextInput } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { Tag } from '../components/StatusPill'
import { PersonRow } from '../components/settings/PersonRow'
import { InviteMemberForm } from '../forms/InviteMemberForm'
import { ProjectForm } from '../forms/ProjectForm'
import { useWorkspace } from '../lib/WorkspaceContext'
import { effectiveRole, projects } from '../lib/workspace'
import {
  invitations as seedInvitations, people as seedPeople, peopleIn,
  type Invitation, type Person,
} from '../lib/people'
import { relativeTime } from '../lib/format'
import { SecurityPolicy } from '../components/settings/SecurityPolicy'
import { ChannelsPanel } from '../components/settings/ChannelsPanel'

type Tab = 'people' | 'projects' | 'security' | 'channels'
type Panel = 'none' | 'invite' | 'new-project'

const CARD = 'mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'

/**
 * Who can reach what, at both levels.
 *
 * The people list is person-centric rather than project-centric because the
 * question an administrator actually arrives with is "what can Dewi see?" —
 * answering it by opening each project in turn is the wrong shape. Each row
 * expands to that person's project grants; the Projects tab covers the other
 * direction.
 */
export function SettingsPage() {
  const [tab, setTab] = useState<Tab>('people')
  const [panel, setPanel] = useState<Panel>('none')
  const [query, setQuery] = useState('')
  const [directory, setDirectory] = useState<Person[]>(seedPeople)
  const [invites, setInvites] = useState<Invitation[]>(seedInvitations)

  const { org, project } = useWorkspace()
  const canAdminOrg = org.role === 'owner' || org.role === 'admin'
  const canAdminProject = effectiveRole(org, project) === 'admin'

  const members = useMemo(() => {
    const all = peopleIn(org, directory)
    const q = query.trim().toLowerCase()
    return q ? all.filter((p) =>
      p.name.toLowerCase().includes(q) || p.email.toLowerCase().includes(q)) : all
  }, [org, directory, query])

  const pending = invites.filter((i) => i.orgId === org.id)

  const update = (id: string, patch: (p: Person) => Person) =>
    setDirectory((d) => d.map((p) => (p.id === id ? patch(p) : p)))

  if (panel !== 'none') {
    const close = () => setPanel('none')
    return (
      <>
        <PageHeader title={panel === 'new-project' ? 'New project' : 'Invite people'} />
        {panel === 'new-project'
          ? <ProjectForm onDone={close} onCancel={close} />
          : <InviteMemberForm scope="organization" scopeName={org.name}
              onDone={close} onCancel={close} />}
      </>
    )
  }

  return (
    <>
      <PageHeader title="Settings" description="Who can reach what, at both levels." />

      <div className="mb-4 flex gap-1 border-b border-line" role="tablist">
        {([['people', 'People'], ['projects', 'Projects'], ['security', 'Security'], ['channels', 'Channels']] as const).map(([id, label]) => (
          <button key={id} type="button" role="tab" aria-selected={tab === id}
            onClick={() => setTab(id)}
            className={`cursor-pointer border-b-2 px-3 py-2.5 text-small font-medium ${
              tab === id ? 'border-accent text-ink'
                : 'border-transparent text-ink-secondary hover:text-ink'}`}>
            {label}
            {id !== 'security' && id !== 'channels' && (
              <span className="ml-1.5 text-caption text-ink-muted">
                {id === 'people' ? members.length : projects.filter((p) => p.orgId === org.id).length}
              </span>
            )}
          </button>
        ))}
      </div>

      {tab === 'people' && (
        <>
          <section className={CARD} data-testid="people-list">
            <div className={HEAD}>
              <div>
                <h2 className="text-lead font-semibold text-ink">People in {org.name}</h2>
                <p className="mt-1 text-small text-ink-secondary">
                  Members see only the projects they are added to. Admins and owners
                  reach every project without being added.
                </p>
              </div>
              <div className="flex items-center gap-2">
                <TextInput size="xs" w={200} placeholder="Search people…" value={query}
                  aria-label="Search people"
                  onChange={(e) => setQuery(e.currentTarget.value)} />
                {canAdminOrg && (
                  <Button onClick={() => setPanel('invite')}>Invite people</Button>
                )}
              </div>
            </div>

            {members.length === 0 ? (
              <p className="px-4 py-10 text-center text-small text-ink-muted">
                Nobody matches “{query}”.
              </p>
            ) : (
              <ul>
                {members.map((p) => (
                  <PersonRow key={p.id} person={p} everyone={directory} canAdmin={canAdminOrg}
                    onChangeOrgRole={(role) =>
                      update(p.id, (x) => ({ ...x, orgRole: role as Person['orgRole'] }))}
                    onChangeProjectRole={(projectId, role) =>
                      update(p.id, (x) => {
                        const next = { ...x.projectRoles }
                        if (role) next[projectId] = role as 'admin' | 'editor' | 'viewer'
                        else delete next[projectId]
                        return { ...x, projectRoles: next }
                      })}
                    onRemove={() => setDirectory((d) => d.filter((x) => x.id !== p.id))} />
                ))}
              </ul>
            )}
          </section>

          {/* Invitations are their own state. A person who has been invited but
              has not accepted is not a member, and listing them together would
              overstate who can actually reach anything. */}
          {pending.length > 0 && (
            <section className={CARD}>
              <div className={HEAD}>
                <div>
                  <h2 className="text-lead font-semibold text-ink">Invited</h2>
                  <p className="mt-1 text-small text-ink-secondary">
                    Nothing changes until they accept.
                  </p>
                </div>
              </div>
              <ul>
                {pending.map((i) => (
                  <li key={i.id}
                    className="flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0">
                    <span className="min-w-[200px] flex-1 font-medium text-ink">{i.email}</span>
                    <Tag>{i.orgRole}</Tag>
                    <span className="text-caption text-ink-muted">
                      Invited {relativeTime(i.invitedAt)} by {i.invitedBy}
                    </span>
                    {canAdminOrg && (
                      <span className="flex gap-1">
                        <Button variant="subtle" size="xs">Resend</Button>
                        <Button variant="subtle" size="xs" color="gray"
                          onClick={() => setInvites((v) => v.filter((x) => x.id !== i.id))}>
                          Cancel
                        </Button>
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          )}
        </>
      )}

      {tab === 'channels' && <ChannelsPanel canAdmin={canAdminOrg} />}

      {tab === 'security' && (
        <SecurityPolicy orgName={org.name} members={members} canAdmin={canAdminOrg}
          twoFactorOn={(p) => !!p.twoFactor} />
      )}

      {tab === 'projects' && (
        <section className={CARD}>
          <div className={HEAD}>
            <div>
              <h2 className="text-lead font-semibold text-ink">Projects in {org.name}</h2>
              <p className="mt-1 text-small text-ink-secondary">
                Each project keeps its own sources, datasets and reports. Nothing
                crosses between them.
              </p>
            </div>
            {canAdminOrg && <Button onClick={() => setPanel('new-project')}>New project</Button>}
          </div>
          {projects.filter((p) => p.orgId === org.id).length === 0 ? (
            <EmptyState title="No projects yet"
              description="A project is an isolation boundary for sources, datasets and reports." />
          ) : (
            <ul>
              {projects.filter((p) => p.orgId === org.id).map((p) => {
                const count = directory.filter((x) =>
                  x.orgId === org.id &&
                  (!!x.projectRoles[p.id] || x.orgRole === 'owner' || x.orgRole === 'admin')).length
                return (
                  <li key={p.id}
                    className="flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0">
                    <span className="min-w-[160px] flex-1 font-medium text-ink">{p.name}</span>
                    <span className="text-small text-ink-secondary">{p.reportCount} reports</span>
                    <span className="text-small text-ink-secondary">
                      {count} {count === 1 ? 'person' : 'people'}
                    </span>
                    <Tag>{effectiveRole(org, p) ?? 'no access'}</Tag>
                  </li>
                )
              })}
            </ul>
          )}
          {!canAdminProject && (
            <p className="border-t border-line px-4 py-3 text-small text-ink-muted">
              You need to be a project admin to change who can reach a project.
              Expand a person on the People tab to manage their access.
            </p>
          )}
        </section>
      )}
    </>
  )
}
