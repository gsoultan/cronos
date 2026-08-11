import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'
import { organizations, projects, type Organization, type Project } from './workspace'

interface Workspace {
  org: Organization
  project: Project
  setContext: (org: Organization, project: Project) => void
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
  const [org, setOrg] = useState<Organization>(organizations[0]!)
  const [project, setProject] = useState<Project>(projects[0]!)

  const value = useMemo<Workspace>(() => ({
    org,
    project,
    setContext: (nextOrg, nextProject) => {
      setOrg(nextOrg)
      setProject(nextProject)
    },
  }), [org, project])

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useWorkspace(): Workspace {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useWorkspace must be used inside WorkspaceProvider')
  return ctx
}
