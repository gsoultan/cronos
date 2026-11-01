import type { CatalogView } from './api'

/**
 * One place ⌘K can take somebody.
 *
 * `to` and `params` are handed to the router as they are, so an entry is a
 * route the portal already serves and never a URL assembled from a name.
 */
export interface JumpEntry {
  key: string
  label: string
  kind: 'Page' | 'Report' | 'Dataset' | 'Source' | 'Schedule'
  to: string
  params?: Record<string, string>
}

/* The pages themselves, so "sett" finds Settings as readily as a report. */
export const PAGES: JumpEntry[] = [
  { key: 'page:reports', label: 'Reports', kind: 'Page', to: '/' },
  { key: 'page:data', label: 'Data', kind: 'Page', to: '/data' },
  { key: 'page:schedules', label: 'Schedules', kind: 'Page', to: '/schedules' },
  { key: 'page:history', label: 'History', kind: 'Page', to: '/activity' },
  { key: 'page:settings', label: 'Settings', kind: 'Page', to: '/settings' },
  { key: 'page:account', label: 'Your account', kind: 'Page', to: '/account' },
]

/** Everything in a catalogue that has a page of its own. */
export function catalogEntries(c: CatalogView): JumpEntry[] {
  return [
    ...c.reports.map((r): JumpEntry => ({
      key: `report:${r.name}`, label: r.title ?? r.name, kind: 'Report',
      to: '/reports/$name', params: { name: r.name },
    })),
    ...c.datasets.map((d): JumpEntry => ({
      key: `dataset:${d.name}`, label: d.title ?? d.name, kind: 'Dataset',
      to: '/data/datasets/$name/edit', params: { name: d.name },
    })),
    ...c.sources.map((s): JumpEntry => ({
      key: `source:${s.name}`, label: s.title ?? s.name, kind: 'Source',
      to: '/data/sources/$name/edit', params: { name: s.name },
    })),
    ...c.schedules.map((s): JumpEntry => ({
      key: `schedule:${s.name}`, label: s.title ?? s.name, kind: 'Schedule',
      to: '/schedules/$name/edit', params: { name: s.name },
    })),
  ]
}

/**
 * The entries that match what was typed, best first.
 *
 * A label that starts with the term beats one that only contains it, and a
 * word inside a label that starts with it beats a match in the middle of a
 * word — "sum" should put "Billing summary" above "Consumption". The
 * identifier is searched too, because it is what is in the address bar and
 * what people paste. Nothing typed means the pages, which is where somebody
 * who opened this without a destination in mind wants to go.
 */
export function matchEntries(entries: JumpEntry[], typed: string, most = 8): JumpEntry[] {
  const term = typed.trim().toLowerCase()
  if (!term) return entries.filter((e) => e.kind === 'Page').slice(0, most)

  const scored: { e: JumpEntry; score: number; i: number }[] = []
  entries.forEach((e, i) => {
    const score = rank(e, term)
    if (score > 0) scored.push({ e, score, i })
  })
  scored.sort((a, b) => b.score - a.score || a.i - b.i)
  return scored.slice(0, most).map((s) => s.e)
}

function rank(e: JumpEntry, term: string): number {
  const label = e.label.toLowerCase()
  if (label.startsWith(term)) return 4
  if (label.split(/[\s\-_/]+/).some((w) => w.startsWith(term))) return 3
  if (label.includes(term)) return 2
  const id = e.params?.name?.toLowerCase()
  if (id?.includes(term)) return 1
  return 0
}
