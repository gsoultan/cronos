/**
 * Sharing a report.
 *
 * The security question is the whole design. A shared report has to be rendered
 * as *somebody*, and there are only two honest answers:
 *
 * - **A snapshot**, rendered as the sender, right now. The recipient sees the
 *   sender's rows. That is what an emailed PDF has always been, and it is fine
 *   as long as it says so.
 * - **A live link**, where the recipient signs in and the dataset's row-level
 *   security applies to *them*. They may legitimately see less, or nothing.
 *
 * What we do not offer is a live link that runs as the sender for anyone who
 * has the URL. That is the combination that leaks: it looks like a convenience
 * and behaves like an unauthenticated export of someone else's data.
 */

export type Channel = 'email' | 'telegram'

export interface ChannelSpec {
  id: Channel
  label: string
  icon: string
  /** What the recipient needs before anything can be delivered. */
  requires: string
  /** Hard limit imposed by the channel itself. */
  sizeLimitMb?: number
}

export const CHANNELS: ChannelSpec[] = [
  {
    id: 'email', label: 'Email', icon: '✉',
    requires: 'An address. Nothing else.',
    sizeLimitMb: 25,
  },
  {
    id: 'telegram', label: 'Telegram', icon: '➤',
    requires: 'The bot must already be in the chat — Telegram will not let it message a stranger.',
    sizeLimitMb: 50,
  },
]

export const channel = (id: Channel) => CHANNELS.find((c) => c.id === id)!

/* -- Links --------------------------------------------------------------- */

export type LinkAudience = 'project' | 'anyone'

export interface LinkOption {
  id: LinkAudience
  label: string
  hint: string
  /** Stated plainly, because this is the part people get wrong. */
  sees: string
  live: boolean
}

export const LINK_AUDIENCES: LinkOption[] = [
  {
    id: 'project',
    label: 'People in this project',
    hint: 'They sign in as themselves.',
    sees: 'Live data, filtered to their own rows. They may see less than you do.',
    live: true,
  },
  {
    id: 'anyone',
    label: 'Anyone with the link',
    hint: 'No sign-in. The link is the credential.',
    sees: 'A snapshot of your rows, frozen when you created the link.',
    live: false,
  },
]

export const EXPIRIES = [
  { value: '1', label: 'After 24 hours' },
  { value: '7', label: 'After 7 days' },
  { value: '30', label: 'After 30 days' },
  { value: '0', label: 'Never' },
]

/* -- Telegram destinations ------------------------------------------------ */

export interface TelegramChat {
  id: string
  title: string
  kind: 'group' | 'channel' | 'direct'
  addedAt: string
}

/** Configured per project; the bot token lives in the secret backend. */
export interface TelegramConnection {
  botUsername: string
  chats: TelegramChat[]
}

export const telegramConnection: TelegramConnection | null = {
  botUsername: '@acme_cronos_bot',
  chats: [
    { id: 'c1', title: 'Finance team', kind: 'group', addedAt: '2026-06-02T09:00:00Z' },
    { id: 'c2', title: 'Month-end alerts', kind: 'channel', addedAt: '2026-07-14T11:30:00Z' },
  ],
}

/* -- Validation ----------------------------------------------------------- */

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export function splitRecipients(raw: string): string[] {
  return raw.split(/[\s,;]+/).map((s) => s.trim()).filter(Boolean)
}

/** Returns the addresses that are not addresses, so the message can name them. */
export function invalidEmails(raw: string): string[] {
  return splitRecipients(raw).filter((r) => !EMAIL.test(r))
}
