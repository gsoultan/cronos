import type { Person } from './people'

/**
 * Two-factor authentication.
 *
 * Three product decisions are encoded here rather than left to a settings page:
 *
 * 1. **No SMS.** SIM-swap makes it the weakest widely-offered second factor,
 *    and it drags in a telephony vendor and a per-message cost to deliver it.
 *    Offering it would let people believe they are protected when they are not.
 * 2. **2FA is in the core, not ee/.** Baseline account security behind a paid
 *    tier is the SSO tax with worse consequences. SSO stays commercial — that
 *    is an enterprise integration, not a floor.
 * 3. **Enrolment cannot complete without verifying a code.** Anything else
 *    enrols people into a lockout.
 */

export type FactorKind = 'app' | 'passkey'

export interface Factor {
  id: string
  kind: FactorKind
  label: string
  addedAt: string
  lastUsed?: string
}

export interface TwoFactorState {
  factors: Factor[]
  /** Shown once at enrolment and never again. */
  recoveryCodesRemaining: number
}

export const METHODS: {
  kind: FactorKind
  label: string
  hint: string
  strength: string
  icon: string
}[] = [
  {
    kind: 'passkey',
    label: 'Passkey',
    hint: 'Face, fingerprint or security key. Nothing to type, nothing to copy.',
    strength: 'Strongest — cannot be phished',
    icon: '⚿',
  },
  {
    kind: 'app',
    label: 'Authenticator app',
    hint: 'A six-digit code from 1Password, Authy, Google Authenticator or similar.',
    strength: 'Strong — works offline',
    icon: '⧉',
  },
]

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

const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'

/*
 * Math.imul, not `*`.
 *
 * A 32-bit LCG written with `*` silently breaks: h is up to 2^32 and the
 * multiplier ~1.1e9, so the product exceeds 2^53 and doubles drop the low bits
 * — which are precisely the bits `% 32` reads. The first version of this
 * produced ten identical recovery codes. Math.imul is a real 32-bit multiply.
 */
function step(h: number): number {
  return (Math.imul(h, 1103515245) + 12345) >>> 0
}

function seedFrom(text: string): number {
  let h = 2166136261
  for (const c of text) h = (Math.imul(h ^ c.charCodeAt(0), 16777619)) >>> 0
  return h
}

/** Deterministic so a re-render does not reissue the secret mid-enrolment. */
export function makeSecret(seed = 'cronos'): string {
  let h = seedFrom(seed)
  let out = ''
  for (let i = 0; i < 32; i++) {
    h = step(h)
    out += ALPHABET[h % ALPHABET.length]
  }
  return out
}

export function formatSecret(secret: string): string {
  return secret.match(/.{1,4}/g)?.join(' ') ?? secret
}

/**
 * Ten single-use codes. Ten because it survives losing a phone more than once
 * without becoming a list nobody stores properly.
 */
export function makeRecoveryCodes(seed: string): string[] {
  let h = seedFrom(`${seed}:recovery`)
  return Array.from({ length: 10 }, () => {
    let code = ''
    for (let i = 0; i < 10; i++) {
      h = step(h)
      code += ALPHABET[h % ALPHABET.length]
      if (i === 4) code += '-'
    }
    return code
  })
}
