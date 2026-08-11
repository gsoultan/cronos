import type { Report } from './types'

/** Mirrors docs/tenancy.md. */

export type OrgRole = 'owner' | 'admin' | 'member'
export type ProjectRole = 'admin' | 'editor' | 'viewer' | null

export interface Project {
  id: string
  slug: string
  name: string
  orgId: string
  /** null when the viewer is an org admin with no explicit membership. */
  role: ProjectRole
  reportCount: number
}

export interface Organization {
  id: string
  slug: string
  name: string
  role: OrgRole
}

/** Org admins reach every project in their org without a project membership. */
export function effectiveRole(org: Organization, project: Project): Exclude<ProjectRole, null> | null {
  if (org.role === 'owner' || org.role === 'admin') return 'admin'
  return project.role
}

export function canEdit(org: Organization, project: Project): boolean {
  const r = effectiveRole(org, project)
  return r === 'admin' || r === 'editor'
}

/* -- Mock workspace ------------------------------------------------------ */

export const organizations: Organization[] = [
  { id: 'o1', slug: 'acme-logistics', name: 'Acme Logistics', role: 'member' },
  { id: 'o2', slug: 'northwind', name: 'Northwind Trading', role: 'admin' },
]

export const projects: Project[] = [
  { id: 'p1', slug: 'finance', name: 'Finance', orgId: 'o1', role: 'editor', reportCount: 2 },
  { id: 'p2', slug: 'operations', name: 'Operations', orgId: 'o1', role: 'viewer', reportCount: 1 },
  { id: 'p3', slug: 'executive', name: 'Executive', orgId: 'o1', role: null, reportCount: 1 },
  { id: 'p4', slug: 'analytics', name: 'Customer analytics', orgId: 'o2', role: 'admin', reportCount: 0 },
  // No explicit membership: reachable only because the viewer is an org admin
  // of Northwind. Exercises the "via org" grant path.
  { id: 'p5', slug: 'billing', name: 'Billing', orgId: 'o2', role: null, reportCount: 0 },
]

/** Which reports a project contains. Folders live inside a project. */
export function reportsIn(all: Report[], project: Project): Report[] {
  const byProject: Record<string, string[]> = {
    p1: ['monthly-invoice-statement', 'overdue-receivables'],
    p2: ['carrier-performance'],
    p3: ['quarterly-revenue'],
    p4: [],
    p5: [],
  }
  const names = byProject[project.id] ?? []
  return all.filter((r) => names.includes(r.name))
}
