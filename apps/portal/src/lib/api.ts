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
 * a devtools console, a screenshot and a support ticket. A session comes from
 * signing in, by password or through the deployment's own directory, and the
 * server enforces its audience either way.
 *
 * # Signing out is two calls, in an order
 *
 * `signOut` forgets the session here. `endSession` is the button: it asks the
 * server where to go next while the token still works, and only then forgets.
 * The involuntary path — a 401 mid-session — takes the first, because by then
 * there is no token left to ask with.
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
export async function signIn(email: string, password: string, code?: string) {
  const base = apiBase()
  if (!base) throw new ApiError(0, 'This portal is not connected to a server.')

  const res = await fetch(base + '/v1/auth/login', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(code ? { email, password, code } : { email, password }),
  })
  if (!res.ok) throw new ApiError(res.status, await serverMessage(res))

  const session = (await res.json()) as {
    token?: string
    expiresIn?: number
    user?: SignedInUser
    factorRequired?: boolean
    mustEnrol?: boolean
  }

  /*
   * The password was right and this account has a second factor.
   *
   * Not an error: nothing was refused. The caller shows a code field and asks
   * again with the same password. This answer is only ever given to somebody
   * who has already proved the password, which is what stops it being a way to
   * learn which accounts are protected.
   */
  if (session.factorRequired) return { factorRequired: true as const }

  if (!session.token || !session.user) {
    throw new ApiError(res.status, 'The server did not return a session.')
  }
  globalThis.localStorage?.setItem('cronos.token', session.token)
  globalThis.localStorage?.setItem('cronos.user', JSON.stringify(session.user))
  /*
   * A session that may only set up a second factor.
   *
   * Kept beside the token because every render has to know: the shell shows the
   * enrolment wizard instead of the app, and without this it would show the app
   * and let every panel discover the restriction by hitting a 403.
   */
  if (session.mustEnrol) globalThis.localStorage?.setItem('cronos.enrol', '1')
  else globalThis.localStorage?.removeItem('cronos.enrol')

  announceSignIn()
  return {
    token: session.token, expiresIn: session.expiresIn ?? 0, user: session.user,
    mustEnrol: session.mustEnrol === true,
  }
}

/** The event the shell listens for, so a session ending is not silent. */
export const SIGNED_OUT = 'cronos:signed-out'

/**
 * And its opposite.
 *
 * Anything holding a copy of who is signed in has to be told when that changes,
 * and until this only the ending was announced. The workspace read the sample
 * directory because it was built before anybody signed in and never looked
 * again — so a connected portal showed somebody's demo organisation, which is
 * plausible enough that nobody questions it.
 */
export const SIGNED_IN = 'cronos:signed-in'

function announceSignIn() {
  globalThis.dispatchEvent?.(new Event(SIGNED_IN))
}

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
  globalThis.localStorage?.removeItem('cronos.enrol')
  globalThis.dispatchEvent?.(new Event(SIGNED_OUT))
}

/**
 * Ends this session and, where there is one, the identity provider's.
 *
 * Clearing our own storage is half a sign-out. Somebody who signed in through
 * their company's directory still has a session there, so signing out and
 * signing back in asks them nothing — which reads as a button that did not
 * work, and on a shared machine means the next person is signed in as the last.
 *
 * Asked before the token is cleared, because the route is authenticated: it is
 * the session being ended that says whose provider session to end. A server
 * that has no SSO, or a provider that publishes no sign-out endpoint, answers
 * with nowhere to go, and the local sign-out is the whole of it.
 */
export async function endSession(): Promise<void> {
  const cfg = apiConfig()
  if (!cfg) {
    signOut()
    return
  }

  let redirect = ''
  try {
    const res = await fetch(cfg.base + '/v1/auth/sso/logout', {
      method: 'POST',
      headers: { authorization: `Bearer ${cfg.token}` },
    })
    // A 404 is a deployment with no SSO configured, which is most of them, and
    // is not a failure to report — there is simply no second session to end.
    if (res.ok) redirect = ((await res.json()) as { redirect?: string }).redirect ?? ''
  } catch {
    // The server is unreachable. Ending the local session is still the right
    // thing and is the part that does not need it; leaving somebody signed in
    // because a network call failed is the worse of the two outcomes.
  }

  signOut()
  if (redirect) globalThis.location?.assign(redirect)
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

/**
 * True when this session exists only to set up a second factor.
 *
 * The project requires one and this account has none. The server enforces it —
 * every route but the enrolment ones answers 403 — and this is how the
 * interface knows to show the wizard rather than a shell of refusals.
 */
export function mustEnrol(): boolean {
  try {
    return globalThis.localStorage?.getItem('cronos.enrol') === '1'
  } catch {
    return false
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
    //
    // Except where the 401 is about the code somebody just typed rather than
    // the session they typed it in. The second-factor routes answer 401 for a
    // wrong or already-used code, with a sentence written to be read — "That
    // code is not right. Check the app is showing the current one." — and
    // signing out here threw the page away before it could be shown. Mistype a
    // digit while enrolling and you were back at the sign-in form with no
    // explanation and the enrolment abandoned; the same for a code that
    // expired between reading it off a phone and pressing the button, which is
    // the ordinary way to be a second late.
    if (!provesACode(path)) signOut()
    throw new ApiError(401, await serverMessage(res))
  }
  if (!res.ok) throw new ApiError(res.status, await serverMessage(res))
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

/**
 * Whether a 401 from this route is about a credential rather than the session.
 *
 * The second-factor routes take a code and check it, so their 401 means the
 * code was wrong — the session that carried it is fine, and ending it is both
 * the wrong conclusion and an unrecoverable one.
 *
 * A list rather than something read off the response, because the server sends
 * the two cases as prose and matching on prose is a translation away from
 * breaking. It is short and it is where the routes are declared, a few lines
 * from the functions that call them.
 *
 * When a factor route 401s because the session really has ended, the page says
 * the code was wrong, which is untrue and harmless: the next call to anything
 * else signs out properly. That is the better way round.
 */
function provesACode(path: string): boolean {
  return path.startsWith('/v1/auth/factor')
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

/**
 * A stored definition, and the version it is at.
 *
 * The version comes back so a save can say what it started from. Without it two
 * people editing one report is a Monday where the second save silently discards
 * the first — the author sees their change land and the other finds theirs gone
 * the next time they open the page.
 */
export interface StoredDefinition {
  yaml: string
  /** The ETag, or empty where a server predates it. */
  version: string
}

export function readDefinition(kind: string, name: string): Promise<StoredDefinition> {
  const cfg = apiConfig()
  if (!cfg) throw new ApiError(0, 'Not connected to a cronos server.')
  return fetch(`${cfg.base}/v1/definitions/${kind}/${name}`, {
    headers: { authorization: `Bearer ${cfg.token}` },
  }).then(async (r) => {
    if (!r.ok) throw new ApiError(r.status, 'Not found.')
    return {
      yaml: await r.text(),
      // Unquoted: the header is a quoted string and what the server compares
      // is the value inside it.
      version: (r.headers.get('ETag') ?? '').replace(/"/g, ''),
    }
  })
}

export interface SignInMethods {
  password: boolean
  sso?: { provider: string }
}

/**
 * Which ways this deployment lets somebody in.
 *
 * The one call made before anybody has a session, and the only unauthenticated
 * one: the page asking is the sign-in page. It learns that a button should
 * exist and nothing else — not the issuer, not whether an address has an
 * account.
 */
export async function signInMethods(): Promise<SignInMethods> {
  const base = apiBase()
  if (!base) return { password: false }
  const r = await fetch(`${base}/v1/auth/methods`)
  if (!r.ok) return { password: true }
  return r.json() as Promise<SignInMethods>
}

/** Where the browser goes to sign in through the deployment's directory. */
export function ssoStart(returning: string): string {
  return `${apiBase()}/v1/auth/sso/start?returning=${encodeURIComponent(returning)}`
}

/**
 * Takes a session out of the URL fragment, if one came back there.
 *
 * The fragment, because a browser does not send it to any server: it reaches
 * no proxy log and no Referer header. Removed from the address bar immediately
 * afterwards, so it survives one render and does not sit in history.
 */
export function adoptSessionFromFragment(): boolean {
  const fragment = globalThis.location?.hash ?? ''
  if (!fragment.startsWith('#token=')) return false

  const token = decodeURIComponent(fragment.slice('#token='.length))
  if (!token) return false

  globalThis.localStorage?.setItem('cronos.token', token)
  globalThis.history?.replaceState(null, '', globalThis.location.pathname + globalThis.location.search)
  announceSignIn()
  return true
}

/** Publishes a definition. The body is YAML, exactly as the author wrote it. */
/**
 * Stores a definition, optionally only if it is still at the version read.
 *
 * `expect` absent means unconditionally, which is what creating something new
 * is. An editor passes what it loaded, and a save built on a version somebody
 * else has already replaced comes back 409 with a sentence naming both.
 */
export function publishDefinition(yaml: string, expect?: string) {
  const cfg = apiConfig()
  if (!cfg) throw new ApiError(0, 'Not connected to a cronos server.')
  return fetch(`${cfg.base}/v1/definitions`, {
    method: 'POST',
    headers: {
      authorization: `Bearer ${cfg.token}`,
      'content-type': 'application/yaml',
      ...(expect ? { 'if-match': `"${expect}"` } : {}),
    },
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
  filters?: ReportFilter[]
  blocks: ReportBlock[]
}

/**
 * One control on a report's filter bar, as the server declares it.
 *
 * `values` is enum only, and is what lets the interface offer a picker rather
 * than a text box somebody has to guess the spelling for.
 */
export interface ReportFilter {
  name: string
  label: string
  type: string
  values?: string[]
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

export interface SendResult {
  sent: string[]
  failed?: Record<string, string>
  bytes: number
}

/**
 * Sends one report, now, to people named here.
 *
 * Rendered once, as the sender: everybody named receives the view of whoever
 * pressed the button. A share link is what to send when they should see their
 * own rows, which is the choice the panel puts beside this one.
 */
export function sendReport(report: string, body: {
  output: string; via: string; to: string[]; subject?: string; note?: string
}) {
  return call<SendResult>(`/v1/reports/${encodeURIComponent(report)}/send`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
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
  return call<{ people: Person[]; canInvite: boolean }>('/v1/people')
}

/**
 * Adds somebody, one of two ways.
 *
 * With no password, an invitation: nothing is created until they accept, and
 * the password is one only they ever see. With one, the account exists
 * immediately and somebody else has chosen their credential — kept because a
 * deployment with no mail server has to be able to add its second
 * administrator somehow.
 *
 * A server that cannot send mail answers 400 to the first form, which is what
 * `invitesAvailable` asks about before showing the choice.
 */
export function addPerson(person: {
  email: string; name: string; role: string; password?: string
}) {
  return call<Person | Invitation>('/v1/people', {
    method: 'POST',
    body: JSON.stringify(person),
  })
}

/** A place held for somebody who has not arrived. */
export interface Invitation {
  id: string
  email: string
  name?: string
  org: string
  project: string
  role: string
  invitedBy?: string
  createdAt: string
  expires: string
}

/** Who has been invited and has not yet accepted. */
export function invitations() {
  return call<Invitation[]>('/v1/people/invitations')
}

/** Withdraws one before it is used. */
export function uninvite(id: string) {
  return call<void>(`/v1/people/invitations/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

/**
 * Who an invitation is for, read without a session.
 *
 * The one call in this module that carries no token — the person making it does
 * not have an account yet, which is the entire point of the endpoint. Its only
 * credential is the secret out of the link, so it goes in a query string on a
 * request the page makes, never in the page's own URL.
 */
export async function describeInvitation(secret: string): Promise<{
  email: string; name?: string; org: string; project: string
  role: string; invitedBy?: string
}> {
  const base = apiBase()
  if (!base) throw new ApiError(0, 'This portal is not connected to a server.')

  const res = await fetch(`${base}/v1/auth/invitation?secret=${encodeURIComponent(secret)}`)
  if (!res.ok) throw new ApiError(res.status, await serverMessage(res))
  return res.json() as Promise<{
    email: string; name?: string; org: string; project: string
    role: string; invitedBy?: string
  }>
}

/**
 * Spends an invitation and signs them in.
 *
 * The session comes back from this call, so there is no login page in between:
 * they have just proved control of the mailbox and chosen a password nobody
 * else has seen, which is more than a sign-in form asks for.
 */
export async function acceptInvitation(secret: string, password: string) {
  const base = apiBase()
  if (!base) throw new ApiError(0, 'This portal is not connected to a server.')

  const res = await fetch(base + '/v1/auth/invitation', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ secret, password }),
  })
  if (!res.ok) throw new ApiError(res.status, await serverMessage(res))

  const out = (await res.json()) as { token?: string; user: SignedInUser }
  if (out.token) {
    globalThis.localStorage?.setItem('cronos.token', out.token)
    globalThis.localStorage?.setItem('cronos.user', JSON.stringify(out.user))
    announceSignIn()
  }
  return out
}

/** Changes a role, or turns access off. Absent fields are left alone. */
export function amendPerson(id: string, change: { role?: string; disabled?: boolean }) {
  return call<void>(`/v1/people/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(change),
  })
}

/**
 * Ends every other session this account has.
 *
 * There is no list of devices to choose from and there cannot be: a portal
 * token is signed and carries no server-side record, so nothing distinguishes
 * one from another. What the server draws is a line in time that every token is
 * checked against, and it hands this browser a fresh one on the right side of
 * it — so the effect is "everywhere else", achieved without pretending to know
 * what a device is.
 */
export async function endOtherSessions(): Promise<void> {
  const replacement = await call<{ token?: string } | undefined>(
    '/v1/auth/sessions/end', { method: 'POST' })

  // Swapped in before the next call goes out. Without this the very next
  // request carries a token the server has just invalidated, and pressing the
  // button would sign this browser out too.
  if (replacement?.token) {
    globalThis.localStorage?.setItem('cronos.token', replacement.token)
  }
}

/**
 * Whether this deployment still needs its first run.
 *
 * Asked without a session, and safely: on a deployment that has been set up the
 * answer is `false` and nothing else. On one that has not, there is nothing to
 * protect yet — the whole point is that no credential exists.
 */
export async function setupNeeded(): Promise<boolean> {
  const base = apiBase()
  if (!base) return false
  try {
    const res = await fetch(base + '/v1/setup')
    if (!res.ok) return false
    return ((await res.json()) as { needed?: boolean }).needed === true
  } catch {
    // Unreachable server. Offering to set up a deployment nobody can talk to
    // ends in a form that fails on submit, so the answer is no.
    return false
  }
}

/**
 * Creates the first account and signs in as it.
 *
 * The one call in this module that both needs no session and hands one back.
 * It works exactly once per deployment: the endpoint closes the moment any
 * account exists, and nothing reopens it.
 */
export async function setUp(first: {
  email: string; name: string; password: string; org: string; project: string
}) {
  const base = apiBase()
  if (!base) throw new ApiError(0, 'This portal is not connected to a server.')

  const res = await fetch(base + '/v1/setup', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(first),
  })
  if (!res.ok) throw new ApiError(res.status, await serverMessage(res))

  const out = (await res.json()) as { token?: string; user: SignedInUser }
  if (out.token) {
    globalThis.localStorage?.setItem('cronos.token', out.token)
    globalThis.localStorage?.setItem('cronos.user', JSON.stringify(out.user))
    announceSignIn()
  }
  return out
}

/* -- Administering the deployment ------------------------------------------
 *
 * Every call below reaches across tenants and needs the platform permission.
 * A caller without it is answered 404 rather than 403, so a portal that shows
 * these to the wrong person shows them empty rather than an error that admits
 * the tier exists.
 *
 * None of them reads a project's data. Opening a report still needs membership
 * in that project — see docs/tenancy.md.
 */

/** Which organisations and projects have people in them. */
export function tenants() {
  return call<{ tenants: Tenant[] }>('/v1/platform/tenants')
}

export interface Tenant {
  org: string
  project: string
  people: number
  disabled: number
}

/** Every account, in every project. */
export function everyPerson() {
  return call<{ people: Person[] }>('/v1/platform/people')
}

/**
 * Creates an account in any organisation and project.
 *
 * Onboarding a customer, which is the one change no project administrator can
 * make — `/v1/people` always creates into the caller's own project, and this
 * names where. The password is chosen rather than emailed: the person has no
 * account, no project and nobody to invite them yet.
 */
export function addAnywhere(person: {
  email: string; name: string; org: string; project: string
  role: string; password: string
}) {
  return call<Person>('/v1/platform/people', {
    method: 'POST',
    body: JSON.stringify(person),
  })
}

/** Moves somebody to another project, or turns their access off. */
export function amendAnywhere(id: string, change: {
  org?: string; project?: string; role?: string; disabled?: boolean
}) {
  return call<void>(`/v1/platform/people/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(change),
  })
}

/** Who administers the deployment. */
export function platformAdmins() {
  return call<{ admins: Person[] }>('/v1/platform/admins')
}

export function grantPlatform(id: string) {
  return call<void>(`/v1/platform/admins/${encodeURIComponent(id)}`, { method: 'POST' })
}

/**
 * Takes it away, and signs that account out.
 *
 * The permission is carried in their token, so it would otherwise outlive the
 * revocation by up to eight hours. The server ends their sessions in the same
 * transaction; refusing the last administrator is its own answer, because a
 * deployment with none cannot make another except from the command line.
 */
export function revokePlatform(id: string) {
  return call<void>(`/v1/platform/admins/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

/**
 * Who this session belongs to.
 *
 * From the server, not from the token: the token carries a subject and a role
 * and nothing anybody would want to read. The account page used to show a name
 * and an address typed into the source, so on a connected deployment it
 * described somebody else entirely — on the page that offers to change this
 * account's password and its second factor.
 */
export function profile() {
  return call<{
    id: string
    email?: string
    name?: string
    org: string
    project: string
    role: string
    createdAt?: string
    account: boolean
  }>('/v1/auth/profile')
}

/**
 * Changes what you are called.
 *
 * The name only. An email is what you sign in with and what an invitation was
 * addressed to, so changing it needs the new address proved before the old one
 * stops working — and half of that shipped is an account nobody can reach.
 */
export function rename(name: string) {
  return call<void>('/v1/auth/profile', {
    method: 'PATCH',
    body: JSON.stringify({ name }),
  })
}

/**
 * What this project requires of the people in it.
 *
 * The counts come back only for an administrator: telling a viewer how many
 * colleagues have no second factor is handing them a list of who to attack.
 */
export function policy() {
  return call<{
    requireTwoFactor: boolean
    covered?: number
    uncovered?: number
  }>('/v1/policy')
}

/** Changes it. Project administrators only. */
export function setPolicy(requireTwoFactor: boolean) {
  return call<{ requireTwoFactor: boolean }>('/v1/policy', {
    method: 'PUT',
    body: JSON.stringify({ requireTwoFactor }),
  })
}

/** What protects this account, without anything that could be used. */
export function factor() {
  return call<{
    enrolled: boolean
    label?: string
    addedAt?: string
    remainingCodes?: number
  }>('/v1/auth/factor')
}

/**
 * Begins enrolment and returns what the app needs.
 *
 * The secret is in this answer and in no other. Nothing is protected yet —
 * the account gains a second factor at `confirmFactor`, when a code computed
 * from this exact secret comes back.
 */
export function startFactor() {
  return call<{ secret: string; uri: string }>('/v1/auth/factor/start', { method: 'POST' })
}

/**
 * Proves the enrolment with a real code and returns the recovery codes.
 *
 * They come back here and nowhere else: they are passwords, shown once, stored
 * as hashes. A page that can fetch them again is a page that hands them to
 * whoever has the session.
 */
export async function confirmFactor(code: string) {
  const out = await call<{ recoveryCodes: string[]; token?: string }>(
    '/v1/auth/factor/confirm', { method: 'POST', body: JSON.stringify({ code }) })

  /*
   * A session that could only enrol has finished.
   *
   * The server hands back an unrestricted token rather than making somebody
   * sign in again with the password and now a code, thirty seconds after
   * proving both.
   */
  if (out.token) {
    globalThis.localStorage?.setItem('cronos.token', out.token)
    globalThis.localStorage?.removeItem('cronos.enrol')
  }
  return out
}

/** Replaces the recovery codes, retiring the old set. */
export function newRecoveryCodes() {
  return call<{ recoveryCodes: string[] }>('/v1/auth/factor/codes', { method: 'POST' })
}

/**
 * Turns the second factor off, with a current code or a recovery code.
 *
 * Proof is required because without it a stolen session strips the factor off
 * the account it stole, at exactly the moment the factor is all that is left.
 */
export function removeFactor(code: string) {
  return call<void>('/v1/auth/factor', {
    method: 'DELETE',
    body: JSON.stringify({ code }),
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
  /**
   * The fields themselves, for the builder.
   *
   * `fields` above is a count, which is what a browsing list wants. The report
   * builder wants the columns, because every control it draws is a choice among
   * them — and until the server sent them it read a fixture instead.
   */
  columns?: CatalogColumn[]
  /** What the dataset takes, so a report can supply it. */
  parameters?: CatalogParameter[]
}

export interface CatalogColumn {
  name: string
  label?: string
  type: string
  role: string
  format?: string
  hidden?: boolean
}

export interface CatalogParameter {
  name: string
  label?: string
  type: string
  required?: boolean
  multiple?: boolean
  values?: string[]
  /** Rendered as text, because that is what a control holds. */
  default?: string
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
  /** What this deployment can deliver through. Empty means nothing can be sent. */
  channels?: string[]
}

export function readCatalog() {
  return call<CatalogView>('/v1/catalog')
}
