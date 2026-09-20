import {
  createContext, useContext, useEffect, useMemo, useState, type ReactNode,
} from 'react'
import {
  organizations, projects,
  type Organization, type OrgRole, type Project, type ProjectRole,
} from './workspace'
import { connected, currentUser, SIGNED_IN } from './api'

/**
 * Per organisation, keyed by org id.
 *
 * The whole value lives here rather than a URL here and the metadata in a
 * component: duplicated state does not switch when the organisation does, and
 * the first version leaked one organisation's logo into another's settings.
 */
export interface Logo {
  url: string
  name: string
  vector: boolean
  width?: number
  height?: number
}

export interface Branding {
  wordmark?: Logo | null
  mark?: Logo | null
}

interface Workspace {
  org: Organization
  project: Project
  setContext: (org: Organization, project: Project) => void
  branding: Branding
  setBranding: (orgId: string, next: Branding) => void
}

const Ctx = createContext<Workspace | null>(null)

/**
 * Holds the active organization and project for the session.
 *
 * There is deliberately no "current project" default resolved from the user
 * record: the context is set explicitly and read explicitly. When this talks to
 * a real API the pair goes into the request path, not a header and not a
 * server-side session — see docs/tenancy.md.
 */
export function WorkspaceProvider({ children }: { children: ReactNode }) {
  /*
   * Connected, this comes from the session and nowhere else.
   *
   * It used to come from the sample directory whatever the deployment, and the
   * default happened to be "Acme Logistics / Finance" — so a connected portal
   * showed a name and a role that were somebody's demo data, and they looked
   * plausible enough that a first run typing those exact names looked correct
   * by coincidence rather than by working.
   *
   * cronos's identity model is one organisation and one project per account, so
   * on a real server there is nothing to switch between and the pair is simply
   * what the token says.
   */
  const [org, setOrg] = useState<Organization>(() => fromSession()?.org ?? organizations[0]!)
  const [project, setProject] = useState<Project>(() => fromSession()?.project ?? projects[0]!)
  const [brandingByOrg, setBrandingByOrg] = useState<Record<string, Branding>>({})

  /* Re-read when a session begins. The provider mounts above the sign-in page,
     so at first render there is nobody signed in and the answer is the sample
     directory — which is right until it is not. */
  useEffect(() => {
    const adopt = () => {
      const real = fromSession()
      if (!real) return
      setOrg(real.org)
      setProject(real.project)
    }
    adopt()
    globalThis.addEventListener(SIGNED_IN, adopt)
    return () => globalThis.removeEventListener(SIGNED_IN, adopt)
  }, [])

  const value = useMemo<Workspace>(() => ({
    org,
    project,
    setContext: (nextOrg, nextProject) => {
      setOrg(nextOrg)
      setProject(nextProject)
    },
    branding: brandingByOrg[org.id] ?? {},
    setBranding: (orgId, next) =>
      setBrandingByOrg((all) => ({ ...all, [orgId]: next })),
  }), [org, project, brandingByOrg])

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useWorkspace(): Workspace {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useWorkspace must be used inside WorkspaceProvider')
  return ctx
}

/**
 * The workspace a real session names, or null on samples.
 *
 * The shapes here carry more than a server knows — a logo, a report count — so
 * the fields it cannot answer are left at their least misleading values rather
 * than invented. A report count of zero beside a project that has reports is a
 * number nobody reads; a fabricated one is a number somebody believes.
 *
 * Exported for its tests and for nothing else: what it turns a session into
 * decides which controls a person is offered, and reading that through a React
 * provider to assert it would be testing the provider.
 */
export function fromSession(): { org: Organization; project: Project } | null {
  if (!connected()) return null

  const me = currentUser()
  if (!me) return null

  /*
   * Both roles, because the server has both and they do not agree.
   *
   * This used to read the project role alone and copy it upwards, on the
   * stated reasoning that a token carries one role and claiming an
   * organisation role would be inventing the half it does not know. The
   * session has carried `orgRole` the whole time. Inventing it was the old
   * behaviour: an owner or an admin of the organisation holds project
   * administrator in every project in it with no membership in any of them —
   * principal's `effective()` is explicit — so somebody who administers the
   * whole organisation was shown, and limited to, the interface of a viewer,
   * while every endpoint behind it would have said yes.
   *
   * The project role stays null rather than defaulting to viewer when the
   * account has none: null is what effectiveRole() reads to mean "no
   * membership here", and a made-up viewer role would hide the org role
   * behind it.
   */
  const orgRole: OrgRole =
    me.orgRole === 'owner' || me.orgRole === 'admin' ? me.orgRole : 'member'
  const role: ProjectRole =
    me.role === 'admin' || me.role === 'editor' || me.role === 'viewer' ? me.role : null

  return {
    org: { id: me.org, slug: me.org, name: me.org, role: orgRole },
    project: {
      id: me.project, slug: me.project, name: me.project,
      orgId: me.org, role, reportCount: 0,
    },
  }
}
