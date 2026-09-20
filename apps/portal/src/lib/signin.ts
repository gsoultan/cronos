/*
 * What the sign-in form can tell somebody before the server does.
 *
 * Only the things a browser can know on its own: a field left empty, and an
 * address that is not an address. Nothing here asks the server anything, so
 * nothing here can leak which addresses have accounts — the one message for
 * every real failure is still the server's, and still the same message.
 */

/*
 * Forgiving on purpose. This decides whether to show a sentence, not whether
 * an address exists, and a stricter pattern only ever refuses real addresses
 * belonging to people who then cannot sign in.
 *
 * The same rule as lib/sharing's, written out again rather than exported from
 * there: that module is the share-link machinery, and importing it here to
 * borrow one regex puts all of it in the first chunk the sign-in page loads.
 */
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export function emailProblem(value: string): string | null {
  const trimmed = value.trim()
  if (trimmed === '') return 'Enter your email address.'
  if (!EMAIL.test(trimmed)) return 'That does not look like an email address.'
  return null
}

/*
 * Empty, and nothing else.
 *
 * A minimum length here reads as helpful and is not. The policy it would
 * enforce is the one for *choosing* a password; applying it at sign-in locks
 * out everybody whose password predates the current policy, and locks them out
 * in the browser, so the server never sees the attempt and nobody can be told
 * why. Whether this password is right is the server's answer and only the
 * server's.
 *
 * Not trimmed, either: a space is a character a password may legitimately
 * start or end with, and trimming one away turns a correct password into a
 * wrong one.
 */
export function passwordProblem(value: string): string | null {
  return value === '' ? 'Enter your password.' : null
}

/*
 * The second factor, which is six digits *or* a recovery code. Emptiness is
 * all that is checked: recovery codes are not six digits, and a length rule
 * here would refuse the one thing somebody reaches for when they have lost the
 * app — at the moment they have least patience for being refused.
 */
export function codeProblem(value: string): string | null {
  return value.trim() === '' ? 'Enter the code from your authenticator app.' : null
}
