import type { FilterValues, ReportPayload } from './types'

/**
 * Fetches a report.
 *
 * The token is opaque here and stays that way. This component never decodes
 * it, never reads a claim out of it, and never decides what a caller may see —
 * it carries it. Every constraint the token pins is enforced where the query
 * is compiled, which is the only place that cannot be edited by whoever opened
 * devtools on the host page.
 *
 * Filters are therefore sent as a request, not applied as a fact. If a token
 * pins a customer, sending a filter for another one returns that caller's own
 * rows or an error — it does not widen anything, because widening is not a
 * thing the client can express.
 */
export class Client {
  constructor(
    private readonly endpoint: string,
    private readonly token: string,
  ) {}

  async report(name: string, filters: FilterValues, signal: AbortSignal): Promise<ReportPayload> {
    const url = `${this.endpoint.replace(/\/$/, '')}/v1/embed/reports/${encodeURIComponent(name)}`
    const res = await fetch(url, {
      method: 'POST',
      signal,
      headers: {
        'authorization': `Bearer ${this.token}`,
        'content-type': 'application/json',
      },
      body: JSON.stringify({ filters }),
    })

    if (!res.ok) throw new Error(await message(res))
    return (await res.json()) as ReportPayload
  }
}

/**
 * Turns a failed response into something a host page can show a person.
 *
 * The server's message is used when it sends one, because it is the only party
 * that knows whether this was an expired token or an unknown report. Where it
 * does not, the status is reported without inventing a cause — "something went
 * wrong" and a guess are equally unhelpful, and the guess is worse when wrong.
 */
async function message(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string; message?: string }
    const said = body.error ?? body.message
    if (said) return said
  } catch {
    // A non-JSON error body is a proxy or a gateway, not the API.
  }
  return res.status === 401 || res.status === 403
    ? 'This report link is no longer valid.'
    : `The report could not be loaded (${res.status}).`
}
