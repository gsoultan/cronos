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

/** Where the API is, or null when the portal is running on samples. */
export function apiConfig(): ApiConfig | null {
  const base = import.meta.env.VITE_CRONOS_API as string | undefined
  const token = (import.meta.env.VITE_CRONOS_TOKEN as string | undefined) ?? readToken()
  if (!base || !token) return null
  return { base: base.replace(/\/$/, ''), token }
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
  periodStart?: string
  periodEnd?: string
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

export function readRun(id: string) {
  return call<{ run: Run; deliveries: RunDelivery[] }>(`/v1/runs/${encodeURIComponent(id)}`)
}
