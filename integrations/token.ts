/*
 * Signing an embed token, on your server.
 *
 * This is the whole of what a host application's backend does for cronos, in
 * whatever language it is written in: a JSON payload, base64url, and an
 * HMAC-SHA256 over it with CRONOS_EMBED_KEY. There is no SDK because there is
 * nothing for one to do.
 *
 *   v1.<base64url(payload)>.<base64url(hmac_sha256(key, "v1.<payload>"))>
 *
 * The key signs embed tokens and nothing else — cronos refuses it for a portal
 * session — but it still decides whose rows a browser sees, so it lives where
 * your other server secrets live and never in a bundle.
 */
import { createHmac } from 'node:crypto'

export interface EmbedClaims {
  /** The organisation and project the report is published in. */
  org: string
  project: string
  /** Who this is in your application. Recorded on the run; never used to decide access. */
  subject: string
  /** The one report this token opens. */
  report: string
  /**
   * The rows this user may read, as the dataset's row-level security names
   * them: `customer_id = {{ .scope.customer_id }}`. Decided here, from your
   * own session — never from anything the browser sent.
   */
  scope: Record<string, string>
}

/** cronos refuses a token that lives longer than this, whoever signed it. */
export const MAX_LIFETIME_SECONDS = 24 * 60 * 60

export function mintEmbedToken(
  key: string,
  claims: EmbedClaims,
  lifetimeSeconds = 15 * 60,
  now = Date.now(),
): { token: string; expiresAt: number } {
  if (Buffer.byteLength(key) < 32) throw new Error('CRONOS_EMBED_KEY must be at least 32 bytes')
  if (lifetimeSeconds <= 0 || lifetimeSeconds > MAX_LIFETIME_SECONDS) {
    throw new Error(`lifetime must be between 1 second and ${MAX_LIFETIME_SECONDS}`)
  }

  const issued = Math.floor(now / 1000)
  const payload = {
    aud: 'embed',
    org: claims.org,
    prj: claims.project,
    sub: claims.subject,
    rpt: claims.report,
    scp: claims.scope,
    // Both are required. A token that does not say when it was issued is
    // refused, so that one signed for ten years cannot hide how long it lives.
    iat: issued,
    exp: issued + lifetimeSeconds,
  }

  const body = 'v1.' + Buffer.from(JSON.stringify(payload)).toString('base64url')
  const mac = createHmac('sha256', key).update(body).digest('base64url')
  return { token: `${body}.${mac}`, expiresAt: payload.exp * 1000 }
}
