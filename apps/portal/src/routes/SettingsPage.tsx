import { useState } from 'react'
import { Button } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { Tag } from '../components/StatusPill'
import { InviteMemberForm } from '../forms/InviteMemberForm'
import { ProjectForm } from '../forms/ProjectForm'
import { useWorkspace } from '../lib/WorkspaceContext'
import { effectiveRole, projects } from '../lib/workspace'

type Panel = 'none' | 'invite-org' | 'invite-project' | 'new-project'

const MEMBERS = [
  { name: 'Dewi Rahayu', email: 'dewi@acme.com', orgRole: 'member', projectRole: 'editor' },
  { name: 'Marek Nowak', email: 'marek@acme.com', orgRole: 'admin', projectRole: null },
  { name: 'Priya Anand', email: 'priya@acme.com', orgRole: 'member', projectRole: 'viewer' },
]

export function SettingsPage() {
  const [panel, setPanel] = useState<Panel>('none')
  const { org, project } = useWorkspace()
  const role = effectiveRole(org, project)
  const canAdminOrg = org.role === 'owner' || org.role === 'admin'
  const canAdminProject = role === 'admin'

  if (panel !== 'none') {
    const close = () => setPanel('none')
    return (
      <>
        <PageHeader eyebrow="Settings"
          title={panel === 'new-project' ? 'New project' : 'Add a member'} />
        {panel === 'new-project'
          ? <ProjectForm onDone={close} onCancel={close} />
          : <InviteMemberForm
              scope={panel === 'invite-org' ? 'organization' : 'project'}
              scopeName={panel === 'invite-org' ? org.name : project.name}
              onDone={close} onCancel={close} />}
      </>
    )
  }

  return (
    <>
      <PageHeader eyebrow={org.name} title="Settings"
        description="Who can reach what, at both levels." />

      <section className="mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card">
        <div className="flex items-center justify-between gap-4 border-b border-line p-4">
          <div>
            <h2 className="text-lead font-semibold text-ink">Projects in {org.name}</h2>
            <p className="mt-1 text-small text-ink-secondary">
              Each project keeps its own sources, datasets and reports. Nothing crosses between them.
            </p>
          </div>
          {canAdminOrg && <Button onClick={() => setPanel('new-project')}>New project</Button>}
        </div>
        <ul>
          {projects.filter((p) => p.orgId === org.id).map((p) => (
            <li key={p.id} className="flex items-center gap-4 border-b border-line px-4 py-3 last:border-b-0">
              <span className="grid flex-1 gap-px font-medium">{p.name}</span>
              <span className="text-small text-ink-secondary">{p.reportCount} reports</span>
              <Tag>{effectiveRole(org, p) ?? 'no access'}</Tag>
            </li>
          ))}
        </ul>
      </section>

      <section className="mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card">
        <div className="flex items-center justify-between gap-4 border-b border-line p-4">
          <div>
            <h2 className="text-lead font-semibold text-ink">People in {project.name}</h2>
            <p className="mt-1 text-small text-ink-secondary">
              Organization admins can reach this project without being listed here.
            </p>
          </div>
          {canAdminProject && (
            <Button variant="default" onClick={() => setPanel('invite-project')}>Add to project</Button>
          )}
        </div>
        <ul>
          {MEMBERS.map((m) => (
            <li key={m.email} className="flex items-center gap-4 border-b border-line px-4 py-3 last:border-b-0">
              <span className="grid flex-1 gap-px font-medium">
                {m.name}
                <span className="text-caption font-normal text-ink-muted">{m.email}</span>
              </span>
              <span className="text-small text-ink-secondary">{m.orgRole} of {org.name}</span>
              <Tag>
                {m.projectRole ?? (m.orgRole === 'admin' ? 'admin · via org' : 'no access')}
              </Tag>
            </li>
          ))}
        </ul>
      </section>

      {canAdminOrg && (
        <section className="mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card">
          <div className="flex items-center justify-between gap-4 border-b border-line p-4">
            <div>
              <h2 className="text-lead font-semibold text-ink">People in {org.name}</h2>
              <p className="mt-1 text-small text-ink-secondary">Members can sign in, but see only the projects they are added to.</p>
            </div>
            <Button variant="default" onClick={() => setPanel('invite-org')}>Invite to organization</Button>
          </div>
        </section>
      )}
    </>
  )
}
