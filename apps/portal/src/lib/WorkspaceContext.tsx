import {
  createContext, useContext, useEffect, useMemo, useState, type ReactNode,
} from 'react'
import {
  organizations, projects, type Organization, type Project, type ProjectRole,
} from './workspace'
import { connected, currentUser, enterableProjects, enterProject, SIGNED_IN } from './api'
import type { EnterableProject } from './api'

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
  /** The projects this session may enter. Empty in sample mode, and on a
      deployment whose build cannot answer — the switcher then shows what the
      sample directory holds, which is what it always showed. */
  enterable: EnterableProject[]
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
   * The pair is what the token says. It used to be all there was to say —
   * an account held one organisation and one project, so on a real server
   * there was nothing to switch between. Memberships changed that: somebody
   * can belong to several projects, and `enterable` is which ones. Switching
   * asks the server for a new session rather than editing this state, because
   * the role in the project being entered is the server's to decide.
   */
  const [org, setOrg] = useState<Organization>(() => fromSession()?.org ?? organizations[0]!)
  const [project, setProject] = useState<Project>(() => fromSession()?.project ?? projects[0]!)
  const [brandingByOrg, setBrandingByOrg] = useState<Record<string, Branding>>({})
  const [enterable, setEnterable] = useState<EnterableProject[]>([])

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

  /* What this session may enter, re-read whenever it changes. Failures are
     swallowed to an empty list on purpose: a switcher that cannot list is a
     switcher with one entry, which is what a deployment without the route has
     anyway — and an error banner across the shell for it would be louder than
     the feature. */
  useEffect(() => {
    let live = true
    const load = () => {
      if (!connected()) return setEnterable([])
      enterableProjects().then((p) => { if (live) setEnterable(p) }).catch(() => {
        if (live) setEnterable([])
      })
    }
    load()
    globalThis.addEventListener(SIGNED_IN, load)
    return () => {
      live = false
      globalThis.removeEventListener(SIGNED_IN, load)
    }
  }, [])

  const value = useMemo<Workspace>(() => ({
    org,
    project,
    enterable,
    setContext: (nextOrg, nextProject) => {
      /* Connected, the server decides. It mints a session naming the project
         and says what role it carries there, so setting this state directly
         would show a role the token does not grant — the screen and the
         requests would disagree, and the screen would be the wrong one. The
         SIGNED_IN it fires brings the new pair back through adopt(). */
      if (connected()) {
        void enterProject(nextProject.slug).catch(() => {})
        return
      }
      setOrg(nextOrg)
      setProject(nextProject)
    },
    branding: brandingByOrg[org.id] ?? {},
    setBranding: (orgId, next) =>
      setBrandingByOrg((all) => ({ ...all, [orgId]: next })),
  }), [org, project, enterable, brandingByOrg])

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
 */
function fromSession(): { org: Organization; project: Project } | null {
  if (!connected()) return null

  const me = currentUser()
  if (!me) return null

  /*
   * A project role, and no organisation role at all.
   *
   * The server's model has both and an account carries one of them: what a
   * token says is a project role. Claiming "member" at the organisation level
   * would be inventing the half it does not know, so the org shows the same
   * role the project does — which is the one that decides anything.
   */
  const role: ProjectRole =
    me.role === 'admin' || me.role === 'editor' || me.role === 'viewer' ? me.role : 'viewer'

  return {
    org: { id: me.org, slug: me.org, name: me.org, role: role === 'admin' ? 'admin' : 'member' },
    project: {
      id: me.project, slug: me.project, name: me.project,
      orgId: me.org, role, reportCount: 0,
    },
  }
}
