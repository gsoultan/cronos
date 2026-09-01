import type { Person } from './people'


/* -- Org policy ---------------------------------------------------------- */

export interface SecurityPolicy {
  requireTwoFactor: boolean
  /** Minutes. 0 means no idle timeout. */
  sessionIdleTimeout: number
}

export interface Readiness {
  covered: number
  total: number
  /** People who would be locked out the moment the requirement turns on. */
  wouldLoseAccess: Person[]
  /** You cannot require what you have not done — you would lock yourself out first. */
  youAreCovered: boolean
}

/**
 * What turning the requirement on would actually do.
 *
 * Enabling a policy blind is how a team locks itself out of its own reporting
 * on a Friday afternoon, so the count and the names come before the switch,
 * not in an email afterwards.
 */
export function readiness(members: Person[], twoFactorOn: (p: Person) => boolean): Readiness {
  const covered = members.filter(twoFactorOn)
  const you = members.find((p) => p.isYou)
  return {
    covered: covered.length,
    total: members.length,
    wouldLoseAccess: members.filter((p) => !twoFactorOn(p)),
    youAreCovered: !!you && twoFactorOn(you),
  }
}

/* -- Enrolment ----------------------------------------------------------- */

export function formatSecret(secret: string): string {
  return secret.match(/.{1,4}/g)?.join(' ') ?? secret
}

