import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { effectiveRole, organizations, projects } from '../lib/workspace'
import type { Organization, Project, ProjectRole } from '../lib/workspace'
import { connected, enterableProjects, enterProject } from '../lib/api'

interface Props {
  org: Organization
  project: Project
  onChange: (org: Organization, project: Project) => void
  /** Icon-rail mode: the trigger shrinks to a badge, the menu keeps its width. */
  collapsed?: boolean
  /** The organisation's square mark, once one has been uploaded. */
  mark?: string
}

/**
 * The projects to offer, with the one this session is in among them.
 *
 * The server answers with a list and a `current` alongside it, and they are
 * not the same question: the list is memberships plus, for an organisation
 * administrator, every project in the organisation — so the account that set
 * the deployment up, which has an organisation role and no membership row
 * anywhere, gets an empty list and a `current` naming exactly where it is. A
 * switcher that drew the list alone told that person they belong to no
 * projects while they were looking at one.
 *
 * The session's own role is used for the entry it adds, because that is what
 * the token says and the list has nothing to say about a project it did not
 * mention.
 */
function withCurrent(
  listed: { project: string; role: string }[],
  current: string | undefined,
  session: Project,
  orgId: string,
): Project[] {
  const out: Project[] = listed.map((p) => ({
    id: p.project, slug: p.project, name: p.project, orgId,
    // Empty means they arrive on their organisation role and hold no
    // membership, which effectiveRole reads as null and labels "via org".
    role: (p.role || null) as ProjectRole,
    reportCount: 0,
  }))

  const here = current ?? session.id
  if (out.some((p) => p.id === here)) return out
  return [{
    id: here, slug: here, name: here, orgId,
    role: session.role, reportCount: 0,
  }, ...out]
}

const ITEM = `grid w-full cursor-pointer grid-cols-[1fr_auto] items-center gap-x-2
  rounded-sm px-2 py-2 text-left text-ink hover:bg-hover`
const HINT = 'col-start-1 text-micro tracking-[0.04em] text-ink-muted uppercase'
const LABEL = 'mx-2 mt-1 mb-1 text-micro font-semibold tracking-[0.06em] text-ink-muted uppercase'

/**
 * The active organization and project, always visible.
 *
 * Two rules from docs/tenancy.md are encoded here: the active context is never
 * inferred, and it is never ambiguous on screen. Acting in the wrong project is
 * the characteristic failure of workspace products, and it happens when the
 * context is a setting somewhere rather than a label you can read.
 *
 * Hand-built rather than a Mantine Menu: this sits in the app shell, so it is
 * in the eager bundle on every page load, and the library's menu costs ~90 KB
 * there — the whole initial-route budget overage on its own.
 */
export function WorkspaceSwitcher({
  org, project, onChange, collapsed = false, mark,
}: Props) {
  const [open, setOpen] = useState(false)
  const [failed, setFailed] = useState<string | null>(null)
  const root = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)

  /*
   * Connected, the list is the server's answer and not this build's fixture.
   *
   * The switcher has always been able to draw this; what it drew was the
   * sample directory, so on a real deployment it offered two organisations
   * nobody belongs to and could not move anybody anywhere. /v1/auth/project
   * has answered both halves — where this session may go, and moving it
   * there — the whole time.
   *
   * Only while the menu is open: this sits in the app shell, so an
   * unconditional query would be a request on every page load for a control
   * most people never touch.
   */
  const live = connected()
  const enterable = useQuery({
    queryKey: ['enterable-projects'],
    queryFn: enterableProjects,
    enabled: live && open,
  })

  useEffect(() => {
    if (!open) return

    function onPointerDown(e: MouseEvent) {
      if (!root.current?.contains(e.target as Node)) setOpen(false)
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        setOpen(false)
        trigger.current?.focus()   // never strand focus in a closed menu
      }
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  /* Only projects this principal can actually enter. A project they have no
     grant on is not shown greyed out — it is not shown, because listing it
     leaks that it exists and offers a click that can only fail. An org member
     with no project memberships correctly sees an empty list.

     Connected, the server applies that same rule and this trusts it: the list
     is their memberships plus, for somebody who administers the organisation,
     every project in it. */
  const mine: Project[] = live
    ? withCurrent(enterable.data?.projects ?? [], enterable.data?.current, project, org.id)
    : projects.filter((p) => p.orgId === org.id && effectiveRole(org, p) !== null)

  /* One organisation connected, because an account belongs to one. Offering
     the fixture's two would be a control that changes which company somebody
     is looking at and then fails. */
  const orgs = live ? [org] : organizations

  async function choose(nextOrg: Organization, nextProject: Project) {
    if (live) {
      if (nextProject.id !== project.id) {
        try {
          setFailed(null)
          /* The token carries the project, so moving means a new one. The
             workspace re-reads itself from the session afterwards, and the
             query cache is emptied on the way — every key in it belongs to
             the project being left. */
          await enterProject(nextProject.id)
        } catch (err) {
          // Left open, with the server's sentence. A switcher that closes and
          // changes nothing is one somebody clicks four more times.
          setFailed(err instanceof Error ? err.message : 'Could not switch project.')
          return
        }
      }
    } else {
      onChange(nextOrg, nextProject)
    }
    setOpen(false)
    trigger.current?.focus()
  }

  /* Switching organization switches project too: a project belongs to exactly
     one organization, and a half-switched context is a bug waiting to happen. */
  function pickOrg(next: Organization) {
    if (next.id === org.id) return setOpen(false)
    const first = projects.find((p) => p.orgId === next.id)
    if (first) choose(next, first)
  }

  /* Arrow keys walk the options; the list is short enough that a roving
     tabindex would be more machinery than it earns. */
  function onListKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    e.preventDefault()
    const items = [...(root.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])]
    const i = items.indexOf(document.activeElement as HTMLButtonElement)
    const next = e.key === 'ArrowDown' ? i + 1 : i - 1
    items[(next + items.length) % items.length]?.focus()
  }

  const label = `${org.name}, ${project.name}. Switch organization or project`

  return (
    <div className={`relative ${collapsed ? '' : 'w-full'}`} ref={root}>
      {collapsed ? (
        /* Two letters of the project carry it in the rail — an org avatar would
           be a second thing to recognise for no extra information. */
        <button type="button" ref={trigger} data-testid="workspace-trigger"
          onClick={() => setOpen((o) => !o)} aria-expanded={open} aria-haspopup="menu"
          aria-label={label} title={`${org.name} · ${project.name}`}
          className="grid size-10 cursor-pointer place-items-center overflow-hidden rounded-md
                     border border-line bg-sunken text-small font-semibold text-ink
                     hover:border-accent">
          {mark
            ? <img src={mark} alt="" className="size-7 object-contain" />
            : project.name.slice(0, 2).toUpperCase()}
        </button>
      ) : (
        <button type="button" ref={trigger} data-testid="workspace-trigger"
          onClick={() => setOpen((o) => !o)} aria-expanded={open} aria-haspopup="menu"
          aria-label={label}
          className="grid w-full cursor-pointer grid-cols-[1fr_auto] items-center gap-x-2
                     rounded-md border border-line bg-sunken px-3 py-2 text-left text-ink
                     transition-colors duration-150 ease-out-quick hover:border-accent">
          <span className="col-start-1 flex items-center gap-1.5 truncate text-caption text-ink-muted">
            {mark && <img src={mark} alt="" className="size-4 shrink-0 object-contain" />}
            {org.name}
          </span>
          <span className="col-start-1 flex min-w-0 items-baseline gap-2 font-semibold">
            <span className="truncate">{project.name}</span>
            <span className="shrink-0 text-micro font-medium tracking-[0.04em] text-ink-muted uppercase">
              {effectiveRole(org, project) ?? 'no access'}
            </span>
          </span>
          <span className="col-start-2 row-span-2 text-ink-muted" aria-hidden>
            {open ? '⌃' : '⌄'}
          </span>
        </button>
      )}

      {open && (
        <div role="menu" data-testid="workspace-menu" onKeyDown={onListKeyDown}
          className={`absolute top-[calc(100%+4px)] z-40 rounded-md border border-line
                      bg-surface p-2 shadow-pop
                      ${collapsed ? 'left-0 w-64' : 'inset-x-0'}`}>
          <p className={LABEL}>Organization</p>
          {orgs.map((o) => (
            <button key={o.id} type="button" role="menuitem" data-testid="workspace-item"
              onClick={() => pickOrg(o)}
              className={`${ITEM} ${o.id === org.id ? 'bg-accent-wash' : ''}`}>
              <span className="col-start-1 text-small font-medium">{o.name}</span>
              <span className={HINT}>{o.role}</span>
              {o.id === org.id && (
                <span className="col-start-2 row-span-2 text-accent" aria-hidden>✓</span>
              )}
            </button>
          ))}

          <p className={LABEL}>Projects in {org.name}</p>
          {mine.map((p) => (
            <button key={p.id} type="button" role="menuitem" data-testid="workspace-item"
              onClick={() => choose(org, p)}
              className={`${ITEM} ${p.id === project.id ? 'bg-accent-wash' : ''}`}>
              <span className="col-start-1 text-small font-medium">{p.name}</span>
              <span className={HINT}>
                {effectiveRole(org, p)}{p.role === null ? ' · via org' : ''}
              </span>
              {p.id === project.id && (
                <span className="col-start-2 row-span-2 text-accent" aria-hidden>✓</span>
              )}
            </button>
          ))}
          {live && enterable.isPending && (
            <p className="m-2 text-small text-ink-muted">Reading…</p>
          )}
          {/* The server's own sentence. "You are in no projects" and "we could
              not ask" are opposite states and both render as an empty list. */}
          {live && enterable.error && (
            <p className="m-2 text-small text-bad" data-testid="workspace-error">
              {enterable.error instanceof Error
                ? enterable.error.message
                : 'Could not read which projects you may open.'}
            </p>
          )}
          {failed && (
            <p className="m-2 text-small text-bad" data-testid="workspace-switch-error">
              {failed}
            </p>
          )}
          {mine.length === 0 && !(live && (enterable.isPending || enterable.error)) && (
            <p className="m-2 text-small text-ink-muted">
              You are a member of {org.name} but have not been added to any of its
              projects. An organization admin can add you.
            </p>
          )}
        </div>
      )}
    </div>
  )
}
