/**
 * Validators return a message or undefined.
 *
 * Messages say what to do, not what is wrong: "Use lower-case letters, numbers
 * and hyphens" beats "Invalid format", which leaves someone guessing at a
 * regex they cannot see.
 */

export type Validator<T = string> = (value: T) => string | undefined

export const required = (what: string): Validator<unknown> => (v) => {
  if (v === undefined || v === null) return `${what} is needed`
  if (typeof v === 'string' && v.trim() === '') return `${what} is needed`
  if (Array.isArray(v) && v.length === 0) return `Choose at least one ${what.toLowerCase()}`
  return undefined
}

export const minLength = (n: number, what: string): Validator => (v) =>
  v && v.trim().length < n ? `${what} needs at least ${n} characters` : undefined

export const maxLength = (n: number, what: string): Validator => (v) =>
  v && v.length > n ? `${what} is too long — keep it under ${n} characters` : undefined

/** Slugs travel in URLs and file paths, so the rule is strict and stated. */
export const slug: Validator = (v) => {
  if (!v) return undefined
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(v)) {
    return 'Use lower-case letters, numbers and hyphens — no spaces'
  }
  return undefined
}

export const email: Validator = (v) => {
  if (!v) return undefined
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v) ? undefined : 'That does not look like an email address'
}

export const url: Validator = (v) => {
  if (!v) return undefined
  try {
    const u = new URL(v)
    return u.protocol === 'http:' || u.protocol === 'https:'
      ? undefined
      : 'Use an http:// or https:// address'
  } catch {
    return 'Use a full address, like https://api.example.com/orders'
  }
}

export const port: Validator<number | undefined> = (v) =>
  v === undefined || (Number.isInteger(v) && v > 0 && v < 65536)
    ? undefined
    : 'Ports run from 1 to 65535'

/**
 * Cron is checked for shape only. The schedule form's plain-English preview is
 * what people actually read to confirm the timing — the expression is there for
 * those who already think in cron.
 */
export const cron: Validator = (v) => {
  if (!v) return undefined
  const parts = v.trim().split(/\s+/)
  if (parts.length !== 5) return 'A schedule has five parts, like 0 6 1 * * — minute, hour, day, month, weekday'
  return undefined
}

/** Runs validators in order and returns the first complaint. */
export function all<T>(...vs: Validator<T>[]): Validator<T> {
  return (value) => {
    for (const v of vs) {
      const msg = v(value)
      if (msg) return msg
    }
    return undefined
  }
}

/** "Monthly invoice statement" → "monthly-invoice-statement" */
export function toSlug(s: string): string {
  return s.toLowerCase().trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 60)
}
