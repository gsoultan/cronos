/**
 * The cronos API, as the portal sees it.
 *
 * # Two modes, and the difference is visible
 *
 * With no `VITE_CRONOS_API` the portal runs on sample data — which is what
 * every browser suite exercises and what makes the interface workable before a
 * server exists. Connected, it shows real numbers.
 *
 * The mode is announced in the shell rather than inferred. A reporting tool
 * showing invented figures that look real is the worst thing this product
 * could do, so "these are samples" is a statement the interface makes rather
 * than something a reader has to deduce from the numbers being suspiciously
 * round.
 *
 * # The token is a portal token
 *
 * Never the admin key. That is a shared secret a deployment pipeline holds,
 * and a browser is the one place it must not be — anything in a browser is in
 * a devtools console, a screenshot and a support ticket. Real sign-in is not
 * built; until it is, the token comes from configuration and the server
 * enforces its audience either way.
 */

export interface ApiConfig {
  base: string
  token: string
}

/** Where the API is, whether or not anybody has signed in. */
export function apiBase(): string | null {
  const base = import.meta.env.VITE_CRONOS_API as string | undefined
  return base ? base.replace(/\/$/, '') : null
}

/** Where the API is, or null when the portal is running on samples. */
export function apiConfig(): ApiConfig | null {
  const base = apiBase()
  const token = (import.meta.env.VITE_CRONOS_TOKEN as string | undefined) ?? readToken()
  if (!base || !token) return null
  return { base, token }
}

/**
 * Signs in and keeps the session.
 *
 * The token goes to localStorage, which is where it lives until there is a
 * cookie to put it in — and it is worth being honest that this is the weaker
 * of the two: a cookie can be HttpOnly and this cannot, so a script that runs
 * on the page can read it. That is a trade taken knowingly for now, and the
 * short lifetime is what limits it.
 */
export async function signIn(email: string, password: string) {
  const base = apiBase()
  if (!base) throw new ApiError(0, 'This portal is not connected to a server.')

  const res = await fetch(base + '/v1/auth/login', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) throw new ApiError(res.status, await serverMessage(res))

  const session = (await res.json()) as { token: string; expiresIn: number; user: SignedInUser }
  globalThis.localStorage?.setItem('cronos.token', session.token)
  globalThis.localStorage?.setItem('cronos.user', JSON.stringify(session.user))
  return session
}

/** The event the shell listens for, so a session ending is not silent. */
export const SIGNED_OUT = 'cronos:signed-out'

/**
 * Forgets the session, and says so.
 *
 * Clearing localStorage is not enough on its own: React has already decided
 * what to render, so a token going stale mid-session would leave the shell up
 * and every panel showing "unauthorised" with no way out. The event is what
 * turns that into the sign-in page.
 */
export function signOut() {
  globalThis.localStorage?.removeItem('cronos.token')
  globalThis.localStorage?.removeItem('cronos.user')
  globalThis.dispatchEvent?.(new Event(SIGNED_OUT))
}

export interface SignedInUser {
  id: string
  email: string
  name?: string
  org: string
  project: string
  role: string
}

/** Who is signed in, as far as the browser knows. */
export function currentUser(): SignedInUser | null {
  try {
    const raw = globalThis.localStorage?.getItem('cronos.user')
    return raw ? (JSON.parse(raw) as SignedInUser) : null
  } catch {
    return null
  }
}

/** True when a server is configured but nobody has signed in. */
export function needsSignIn(): boolean {
  return apiBase() !== null && apiConfig() === null
}

/**
 * A token stashed by whoever signed in.
 *
 * localStorage is where a token lives until there is a session cookie to put
 * it in. Guarded because this module is imported by tests that run without a
 * DOM, and a throw here would take the whole app down before it rendered.
 */
function readToken(): string | undefined {
  try {
    return globalThis.localStorage?.getItem('cronos.token') ?? undefined
  } catch {
    return undefined
  }
}

/** True when the portal is talking to a real cronos. */
export function connected(): boolean {
  return apiConfig() !== null
}

export class ApiError extends Error {
  constructor(readonly status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

async function call<T>(path: string, init: RequestInit = {}): Promise<T> {
  const cfg = apiConfig()
  if (!cfg) throw new ApiError(0, 'Not connected to a cronos server.')

  const res = await fetch(cfg.base + path, {
    ...init,
    headers: {
      authorization: `Bearer ${cfg.token}`,
      'content-type': 'application/json',
      ...init.headers,
    },
  })

  if (res.status === 401) {
    // The session is over. Clearing it here rather than at each call site is
    // what makes the next render show the sign-in page instead of an error
    // nobody can act on — a portal that says "unauthorised" and offers no way
    // to fix it is a portal somebody reloads until it works.
    signOut()
    throw new ApiError(401, await serverMessage(res))
  }
  if (!res.ok) throw new ApiError(res.status, await serverMessage(res))
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

/**
 * The server's own sentence, when it sent one.
 *
 * It is the only party that knows whether this was an expired token or a
 * report that does not exist, and its messages are written for a person.
 * Inventing one here would mean two vocabularies for the same failures.
 */
async function serverMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string }
    if (body.error) return body.error
  } catch {
    // A non-JSON error body is a proxy, not the API.
  }
  return res.status === 401 || res.status === 403
    ? 'You are not signed in to this project.'
    : `The server returned ${res.status}.`
}

/* -- What the portal asks for --------------------------------------------- */

export interface DefinitionEntry {
  kind: string
  name: string
  version: string
}

export function listDefinitions() {
  return call<{ definitions: DefinitionEntry[] }>('/v1/definitions')
}

export function readDefinition(kind: string, name: string) {
  const cfg = apiConfig()
  if (!cfg) throw new ApiError(0, 'Not connected to a cronos server.')
  return fetch(`${cfg.base}/v1/definitions/${kind}/${name}`, {
    headers: { authorization: `Bearer ${cfg.token}` },
  }).then((r) => (r.ok ? r.text() : Promise.reject(new ApiError(r.status, 'Not found.'))))
}

/** Publishes a definition. The body is YAML, exactly as the author wrote it. */
export function publishDefinition(yaml: string) {
  const cfg = apiConfig()
  if (!cfg) throw new ApiError(0, 'Not connected to a cronos server.')
  return fetch(`${cfg.base}/v1/definitions`, {
    method: 'POST',
    headers: { authorization: `Bearer ${cfg.token}`, 'content-type': 'application/yaml' },
    body: yaml,
  }).then(async (r) => {
    if (!r.ok) throw new ApiError(r.status, await serverMessage(r))
    return (await r.json()) as DefinitionEntry
  })
}

export function deleteDefinition(kind: string, name: string) {
  return call<void>(`/v1/definitions/${kind}/${name}`, { method: 'DELETE' })
}

export interface ReportView {
  title: string
  description?: string
  filters?: { name: string; label: string; type: string }[]
  blocks: ReportBlock[]
}

export interface ReportBlock {
  kind: 'stat' | 'chart' | 'table' | 'text'
  title: string
  value?: string
  chart?: string
  series?: { label: string; value: number; formatted: string }[]
  columns?: { label: string; align?: 'left' | 'right' }[]
  rows?: string[][]
  total?: number
  coverage?: { applied?: string[]; ignored?: string[] }
}

export interface RunFilters {
  [name: string]: { op: string; values: unknown[] }
}

export function runReport(name: string, body: { filters?: RunFilters; params?: Record<string, unknown> }) {
  return call<ReportView>(`/v1/reports/${encodeURIComponent(name)}`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export interface Run {
  id: string
  schedule: string
  report: string
  reportVersion?: string
  output?: string
  periodStart?: string
  periodEnd?: string
  /** The scheduler, or the person who asked for it now. */
  triggeredBy?: string
  startedAt: string
  finishedAt?: string
  recipients: number
  delivered: number
  status: 'running' | 'delivered' | 'partial' | 'failed'
  error?: string
}

export function listRuns(limit = 50) {
  return call<{ runs: Run[] }>(`/v1/runs?limit=${limit}`)
}

export interface RunDelivery {
  recipient: string
  channel: string
  destination: string
  filename?: string
  status: string
  attempts: number
  bytes: number
  error?: string
}

/**
 * Runs one schedule now.
 *
 * The only way to find out whether a monthly schedule works without waiting
 * for the first of the month. Accepted rather than completed: a burst of five
 * thousand documents outlives the request, and what it delivered appears in
 * the run history rather than in this response.
 */
export function runSchedule(name: string) {
  return call<{ schedule: string; status: string }>(
    `/v1/schedules/${encodeURIComponent(name)}/run`, { method: 'POST' })
}

/* ---------------------------------------------------------------------- *
 * Sharing: a report handed to somebody who is not in the project.
 * ---------------------------------------------------------------------- */

export interface Share {
  id: string
  report: string
  state: 'live' | 'expired' | 'revoked'
  createdBy: string
  createdAt: string
  expiresAt?: string
  revokedAt?: string
  scoped?: boolean
}

/** Records a link to one report. Days of 0 means it does not expire. */
export function createShare(report: string, days: number) {
  return call<Share>('/v1/shares', {
    method: 'POST',
    body: JSON.stringify({ report, days }),
  })
}

export interface ProbeResult {
  source: string
  ok: boolean
  /** How long it took to answer. A slow source is not a healthy one. */
  ms: number
  error?: string
}

/**
 * Asks a datasource whether it is there.
 *
 * Two hundred either way: the probe ran, and this is what it found. A failure
 * carries the driver's own sentence, which is the only thing that says whether
 * it was a wrong password, a closed port or a database that does not exist.
 */
export function testDataSource(name: string) {
  return call<ProbeResult>(`/v1/datasources/${encodeURIComponent(name)}/test`, { method: 'POST' })
}

export function listShares() {
  return call<{ shares: Share[] }>('/v1/shares')
}

/** Withdraws a link. Takes effect on the next request, not on the next expiry. */
export function revokeShare(id: string) {
  return call<void>(`/v1/shares/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

/**
 * Opens a share, and reads the report behind it.
 *
 * The only pair of calls in this file that send no session: whoever follows a
 * share link is not signed in and has no identity here. The link is exchanged
 * for a short-lived token pinned to one report, and that token — not the
 * session — is what reads it.
 */
export async function openShare(id: string): Promise<{ token: string; report: string }> {
  const base = apiBase()
  if (!base) throw new ApiError(0, 'This portal is not connected to a server.')

  const r = await fetch(`${base}/v1/shares/${encodeURIComponent(id)}/open`, { method: 'POST' })
  if (!r.ok) throw new ApiError(r.status, await serverMessage(r))
  return r.json() as Promise<{ token: string; report: string }>
}

/** Renders a shared report with the token that opening it returned. */
export async function readShared(token: string, report: string): Promise<ReportView> {
  const base = apiBase()
  if (!base) throw new ApiError(0, 'This portal is not connected to a server.')

  const r = await fetch(`${base}/v1/embed/reports/${encodeURIComponent(report)}`, {
    method: 'POST',
    headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json' },
    body: '{}',
  })
  if (!r.ok) throw new ApiError(r.status, await serverMessage(r))
  return r.json() as Promise<ReportView>
}

/* ---------------------------------------------------------------------- *
 * Who has access.
 *
 * One org, one project and one role per person — which is what the server
 * models, and less than the sample directory shows. The sample one describes
 * an organisation with several projects and a person in some of them; the
 * server has never had a membership table. Rather than have the interface
 * promise the richer shape, the connected view shows what is actually true.
 * ---------------------------------------------------------------------- */

export interface Person {
  id: string
  email: string
  name?: string
  org: string
  project: string
  role: 'admin' | 'editor' | 'viewer'
  createdAt: string
  lastSeen?: string
  disabled?: boolean
}

export function listPeople() {
  return call<{ people: Person[] }>('/v1/people')
}

/**
 * Adds somebody, with a password to hand them.
 *
 * Not an emailed invitation: that needs a token, a delivery channel that is
 * configured, and a set-password page that works with no session — and half of
 * that shipped is a link that does not open.
 */
export function addPerson(person: {
  email: string; name: string; role: string; password: string
}) {
  return call<Person>('/v1/people', { method: 'POST', body: JSON.stringify(person) })
}

/** Changes a role, or turns access off. Absent fields are left alone. */
export function amendPerson(id: string, change: { role?: string; disabled?: boolean }) {
  return call<void>(`/v1/people/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(change),
  })
}

/** Changes your own password, having proved you know the current one. */
export function changePassword(current: string, next: string) {
  return call<void>('/v1/auth/password', {
    method: 'POST',
    body: JSON.stringify({ current, new: next }),
  })
}

export function readRun(id: string) {
  return call<{ run: Run; deliveries: RunDelivery[] }>(`/v1/runs/${encodeURIComponent(id)}`)
}

/* -- The catalogue: what the project contains ----------------------------- */

export interface SourceSummary {
  name: string
  /** What a person calls it. The name is the identifier. */
  title?: string
  description?: string
  driver: string
  detail?: string
  datasets: number
  federated?: boolean
  maxRows: number
  timeout: string
}

export interface DatasetSummary {
  name: string
  /** What a person calls it. The name is the identifier. */
  title?: string
  description?: string
  sources: string[]
  fields: number
  measures: number
  params: number
  rowScoped: boolean
}

export interface ReportSummary {
  name: string
  title?: string
  description?: string
  folder?: string
  datasets: string[]
  outputs: string[]
  blocks: number
}

export interface ScheduleSummary {
  name: string
  /** What a person calls it. The name is the identifier. */
  title?: string
  description?: string
  report: string
  output: string
  cron: string
  timezone: string
  bursts: boolean
  over?: string
  channels: string[]
  /** Absent when no scheduler is armed here — see the schedules page. */
  next?: string
}

export interface CatalogView {
  sources: SourceSummary[]
  datasets: DatasetSummary[]
  reports: ReportSummary[]
  schedules: ScheduleSummary[]
}

export function readCatalog() {
  return call<CatalogView>('/v1/catalog')
}
