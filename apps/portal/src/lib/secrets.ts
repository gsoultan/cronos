import type { SecretEntry, SecretList, SecretSource, SecretUse } from './types'

/**
 * Secrets, as the portal names and checks them.
 *
 * A secret is what a definition's `${secret:name}` resolves to: a warehouse
 * password, a bucket's keys, a Mapbox token. The definition names it and never
 * holds it, because a definition is a file somebody commits and a password in
 * one is in their history for ever.
 *
 * The rules below are the server's — internal/platform/secret for what a name
 * may be, internal/app/vault for what a value may be — restated for the reason
 * maps.ts restates the map rules: they decide what a form offers and when its
 * button works. A form that let any name through would find out from a 422
 * after somebody had pasted a token, and one that stored a name the server
 * resolves differently would store something nothing reads. The server keeps
 * the last word, and says it in a sentence the forms show whole.
 */

/**
 * What a name may be: what `${secret:…}` accepts between its braces, starting
 * with a letter or a digit so it can never be `.` or `..` — which a secret
 * read from a directory of files would treat as a directory.
 */
const NAME = /^[A-Za-z0-9][A-Za-z0-9_.-]*$/

/** Every reference in a string, as the server finds them — `|url` is how a
 *  definition says the value sits in a URL and is to be encoded there. */
const REFERENCE = /\$\{secret:([A-Za-z0-9_.-]+)(?:\|url)?\}/g

/** A string that is one reference and nothing else. */
const WHOLE = /^\$\{secret:([A-Za-z0-9_.-]+)(?:\|url)?\}$/

export const MAX_NAME = 128

/** 16 KiB, measured in bytes as the server measures it, not in characters. */
export const MAX_VALUE_BYTES = 16 * 1024

/** Why a name cannot be stored, or nothing when it can. */
export function secretNameProblem(name: string): string | undefined {
  if (name === '') return 'A name is needed'
  if (name.length > MAX_NAME) return `Keep it to ${MAX_NAME} characters or fewer`
  if (!NAME.test(name)) {
    return 'Use letters, digits, dots, dashes and underscores, starting with a letter or a digit'
  }
  return undefined
}

/**
 * Why a value cannot be stored, or nothing when it can.
 *
 * Measured after the line ending is gone, because the server trims it first:
 * pasting from a file or a terminal adds one, and a password that silently
 * gained a newline is a password that no longer works.
 */
export function secretValueProblem(value: string): string | undefined {
  const kept = value.replace(/[\r\n]+$/, '')
  if (kept === '') return 'A value is needed — to take one away, remove the secret instead'
  if (new TextEncoder().encode(kept).length > MAX_VALUE_BYTES) {
    return 'That is longer than the 16 KB a secret may hold'
  }
  return undefined
}

/**
 * The environment variable a deployment answers a name from.
 *
 * Upper-cased, with dots and dashes as underscores, so that
 * `warehouse-password` is CRONOS_SECRET_WAREHOUSE_PASSWORD — a variable
 * somebody can actually set. secret.Env on the server is the other copy.
 */
export function envName(name: string): string {
  return `CRONOS_SECRET_${name.toUpperCase().replace(/[.-]/g, '_')}`
}

/**
 * `${secret:name}`, the only way a definition ever mentions one — or
 * `${secret:name|url}` where it sits inside a URL, which the server
 * percent-encodes as it substitutes. The value stored is then the password
 * itself, whatever it holds: a space, an @ or a / no longer ends the userinfo
 * early, and the same secret reads correctly anywhere else it is named.
 */
export function reference(name: string, inUrl = false): string {
  return inUrl ? `\${secret:${name}|url}` : `\${secret:${name}}`
}

/** The name a value refers to, when the whole value is one reference. */
export function referenced(value: string | undefined): string | undefined {
  return value ? WHOLE.exec(value.trim())?.[1] : undefined
}

/** Every name a string refers to, once each, in the order they appear. */
export function referencedNames(value: string | undefined): string[] {
  const out: string[] = []
  for (const m of (value ?? '').matchAll(REFERENCE)) {
    if (m[1] && !out.includes(m[1])) out.push(m[1])
  }
  return out
}

/**
 * What a datasource's own secrets are called.
 *
 * Its API name and what the secret is, joined by an underscore. For the
 * password that is the name every connection string this portal has ever
 * written already names, so a source connected before anything could store
 * its password is fixed by storing one under the name it was always reading;
 * a bucket's credentials follow the same pattern.
 */
export function sourceSecret(slug: string, what: 'password' | 'credentials'): string {
  return `${slug}_${what}`
}

/** The entry for a name, when the server listed one. */
export function entryFor(list: SecretList | undefined, name: string): SecretEntry | undefined {
  return list?.secrets.find((s) => s.name === name)
}

/** What each source is called on screen. Said in words: a colour alone says nothing. */
export const SOURCE_LABEL: Record<SecretSource, string> = {
  project: 'Stored in this project',
  deployment: 'From the deployment',
  missing: 'Missing',
}

/** "a", "a and b", "a, b and c". */
export function listed(items: string[]): string {
  return items.length < 2 ? items.join('') : `${items.slice(0, -1).join(', ')} and ${items.at(-1)}`
}

/**
 * What stops working when nothing answers a name, said per kind of thing.
 *
 * A source that cannot resolve its password cannot connect, and a map without
 * its key is drawn with no basemap — two very different afternoons, so the
 * sentence names which one this is rather than calling both "broken".
 */
export function brokenWithout(uses: SecretUse[]): string {
  const sources = uses.filter((u) => u.kind === 'DataSource').map((u) => u.name)
  const reports = uses.filter((u) => u.kind === 'Report').map((u) => u.name)
  const parts: string[] = []
  if (sources.length > 0) {
    parts.push(`${listed(sources)} cannot connect, so the reports reading `
      + `${sources.length === 1 ? 'it' : 'them'} fail`)
  }
  if (reports.length > 0) {
    parts.push(`${listed(reports)} ${reports.length === 1 ? 'draws its' : 'draw their'} maps without a basemap`)
  }
  return parts.join('; ')
}

/**
 * What removing a stored secret does, said before it is done.
 *
 * Conditional, because it is: a project's own value sits in front of the
 * deployment's, and whether the deployment has one behind it is something the
 * server does not say about a name the project stores. Claiming either answer
 * here would be a guess presented as a fact.
 */
export function removalConsequence(entry: SecretEntry): string {
  if (entry.usedBy.length === 0) return 'Nothing uses it.'
  const who = listed(entry.usedBy.map((u) => u.name))
  return `${who} will fall back to the server’s own ${entry.name} if its environment sets one. `
    + `If it does not, ${brokenWithout(entry.usedBy)}.`
}

/**
 * What storing one did.
 *
 * Nothing needs restarting and the sentence says so, because the question
 * somebody has after setting a warehouse password is whether they now have to
 * ask for a deploy.
 */
export function afterSetting(name: string, entry: SecretEntry | undefined): string {
  const uses = entry?.usedBy ?? []
  if (uses.length === 0) {
    return `Stored ${name}. Nothing reads it yet — a definition names it as ${reference(name)}.`
  }
  const who = listed(uses.map((u) => u.name))
  return `Stored ${name}. ${who} ${uses.length === 1 ? 'reads' : 'read'} it from now on; `
    + 'nothing needs restarting.'
}

/**
 * What removing one did, once the server has said where the name stands now.
 *
 * Read from the list after the change rather than predicted before it, so the
 * sentence is the server's answer rather than this module's hope.
 */
export function afterRemoval(name: string, now: SecretEntry | undefined): string {
  if (!now || now.usedBy.length === 0) return `Removed ${name}.`
  const who = listed(now.usedBy.map((u) => u.name))
  if (now.source === 'deployment') {
    return `Removed ${name}. ${who} now ${now.usedBy.length === 1 ? 'reads' : 'read'} the `
      + 'server’s own value.'
  }
  return `Removed ${name}. Nothing answers it now: ${brokenWithout(now.usedBy)} until it is set again.`
}
