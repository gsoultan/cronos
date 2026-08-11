import type { OrgRole, Organization, Project, ProjectRole } from './workspace'
import { projects } from './workspace'

export interface Person {
  id: string
  name: string
  email: string
  orgId: string
  orgRole: OrgRole
  /** Explicit project grants. Org admins reach everything regardless. */
  projectRoles: Record<string, Exclude<ProjectRole, null>>
  /** The signed-in person, so their own row can be marked and protected. */
  isYou?: boolean
  lastActive?: string
}

export interface Invitation {
  id: string
  email: string
  orgId: string
  orgRole: OrgRole
  invitedAt: string
  invitedBy: string
}

export const ORG_ROLES: { value: OrgRole; label: string; hint: string }[] = [
  { value: 'member', label: 'Member', hint: 'Sees only the projects they are added to' },
  { value: 'admin', label: 'Admin', hint: 'Manages people and projects, and reaches every project' },
  { value: 'owner', label: 'Owner', hint: 'Everything an admin can do, plus billing' },
]

export const PROJECT_ROLES: { value: Exclude<ProjectRole, null>; label: string; hint: string }[] = [
  { value: 'viewer', label: 'Viewer', hint: 'Runs and views reports' },
  { value: 'editor', label: 'Editor', hint: 'Creates and edits datasets, reports and schedules' },
  { value: 'admin', label: 'Admin', hint: 'Also manages members, sources and settings' },
]

/* -- Mock directory ------------------------------------------------------ */

/*
 * The directory.
 *
 * `orgRole` for the person marked `isYou` MUST match that organisation's `role`
 * in workspace.ts — they are the same fact, and the first version of this file
 * disagreed with itself, which showed up as an admin screen rendering read-only.
 * Acme has you as a member (so Settings is read-only there, which is worth
 * seeing); Northwind has you as an admin.
 */
export const people: Person[] = [
  // Acme Logistics — you are a member here.
  {
    id: 'u1', name: 'Nadia Haq', email: 'nadia@acme.com', orgId: 'o1', orgRole: 'owner',
    projectRoles: {}, lastActive: '2026-08-11T06:02:00Z',
  },
  {
    id: 'u2', name: 'Marek Nowak', email: 'marek@acme.com', orgId: 'o1', orgRole: 'admin',
    projectRoles: {}, lastActive: '2026-08-10T16:05:00Z',
  },
  {
    id: 'u3', name: 'Dewi Rahayu', email: 'dewi@acme.com', orgId: 'o1', orgRole: 'member',
    projectRoles: { p1: 'editor', p2: 'viewer' }, isYou: true, lastActive: '2026-08-11T05:20:00Z',
  },
  {
    id: 'u4', name: 'Priya Anand', email: 'priya@acme.com', orgId: 'o1', orgRole: 'member',
    projectRoles: { p1: 'viewer' }, lastActive: '2026-08-11T04:41:00Z',
  },
  {
    id: 'u5', name: 'Tomas Berg', email: 'tomas@acme.com', orgId: 'o1', orgRole: 'member',
    projectRoles: { p2: 'viewer', p3: 'viewer' }, lastActive: '2026-07-29T11:12:00Z',
  },
  {
    id: 'u6', name: 'Aisha Karim', email: 'aisha@acme.com', orgId: 'o1', orgRole: 'member',
    projectRoles: {},
  },

  // Northwind Trading — you are an admin here, so the management path is live.
  {
    id: 'u7', name: 'Dewi Rahayu', email: 'dewi@acme.com', orgId: 'o2', orgRole: 'admin',
    projectRoles: {}, isYou: true, lastActive: '2026-08-11T05:20:00Z',
  },
  {
    id: 'u8', name: 'Ravi Menon', email: 'ravi@northwind.example', orgId: 'o2', orgRole: 'owner',
    projectRoles: {}, lastActive: '2026-08-08T14:30:00Z',
  },
  {
    id: 'u9', name: 'Lena Fischer', email: 'lena@northwind.example', orgId: 'o2', orgRole: 'member',
    projectRoles: { p4: 'editor' }, lastActive: '2026-08-11T03:15:00Z',
  },
  {
    id: 'u10', name: 'Owen Pratt', email: 'owen@northwind.example', orgId: 'o2', orgRole: 'member',
    projectRoles: {},
  },
]

export const invitations: Invitation[] = [
  {
    id: 'i1', email: 'sam@northwind.example', orgId: 'o2', orgRole: 'member',
    invitedAt: '2026-08-09T10:00:00Z', invitedBy: 'Dewi Rahayu',
  },
]

/* -- Rules --------------------------------------------------------------- */

/**
 * An organization must keep at least one owner.
 *
 * Enforced by disabling the option with a stated reason rather than by
 * rejecting the change afterwards: an admin who can select "Member" on the last
 * owner and only then be told no has already been misled by the interface.
 */
export function isLastOwner(person: Person, all: Person[]): boolean {
  if (person.orgRole !== 'owner') return false
  return all.filter((p) => p.orgId === person.orgId && p.orgRole === 'owner').length === 1
}

/** Org owners and admins reach every project without an explicit grant. */
export function reachesAllProjects(person: Person): boolean {
  return person.orgRole === 'owner' || person.orgRole === 'admin'
}

/**
 * What a person can actually open, which is not the same as what they were
 * granted — showing only explicit grants would render an org admin as having
 * no access at all.
 */
export function projectsFor(person: Person): { project: Project; role: string; viaOrg: boolean }[] {
  const inOrg = projects.filter((p) => p.orgId === person.orgId)
  if (reachesAllProjects(person)) {
    return inOrg.map((project) => ({ project, role: 'admin', viaOrg: true }))
  }
  return inOrg
    .filter((p) => person.projectRoles[p.id])
    .map((project) => ({ project, role: person.projectRoles[project.id]!, viaOrg: false }))
}

export function peopleIn(org: Organization, all: Person[]): Person[] {
  return all.filter((p) => p.orgId === org.id)
}

export function peopleInProject(project: Project, all: Person[]): Person[] {
  return all.filter((p) => p.orgId === project.orgId &&
    (!!p.projectRoles[project.id] || reachesAllProjects(p)))
}
