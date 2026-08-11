import { Button } from '@mantine/core'
import { Tag } from '../StatusPill'
import { relativeTime } from '../../lib/format'
import { CHANNELS, telegramConnection } from '../../lib/sharing'

const CARD = 'mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'

/**
 * Where a report can be sent.
 *
 * Email needs nothing configured. Telegram does, and the reason is worth
 * stating rather than presenting as a setup chore: the Bot API refuses to
 * message anyone who has not added the bot first. That is a deliberate
 * anti-spam rule, it cannot be worked around, and a person hunting for "why
 * can I not just type a username" deserves to be told once.
 */
export function ChannelsPanel({ canAdmin }: { canAdmin: boolean }) {
  return (
    <>
      {CHANNELS.map((c) => (
        <section key={c.id} className={CARD} data-testid={`channel-panel-${c.id}`}>
          <div className={HEAD}>
            <div>
              <h2 className="flex items-center gap-2 text-lead font-semibold text-ink">
                <span aria-hidden className="text-accent">{c.icon}</span>
                {c.label}
                {c.id === 'email' || telegramConnection
                  ? <span className="rounded-full bg-good/15 px-2 py-px text-micro font-medium text-delta-good">Connected</span>
                  : <span className="rounded-full bg-serious/20 px-2 py-px text-micro font-medium text-ink">Not connected</span>}
              </h2>
              <p className="mt-1 max-w-[68ch] text-small text-ink-secondary">
                {c.requires}
                {c.sizeLimitMb && ` Files over ${c.sizeLimitMb} MB are rejected by ${c.label}.`}
              </p>
            </div>
            {c.id === 'telegram' && canAdmin && (
              <Button variant="default">{telegramConnection ? 'Reconnect' : 'Connect'}</Button>
            )}
          </div>

          {c.id === 'telegram' && telegramConnection && (
            <>
              <p className="border-b border-line px-4 py-3 text-small text-ink-secondary">
                Bot <code className="font-mono text-ink">{telegramConnection.botUsername}</code>.
                Add it to a group or channel and it appears below — Telegram will not
                let a bot start a conversation, so this step cannot be skipped.
              </p>
              <ul>
                {telegramConnection.chats.map((chat) => (
                  <li key={chat.id}
                    className="flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0">
                    <span className="min-w-[200px] flex-1 font-medium text-ink">{chat.title}</span>
                    <Tag>{chat.kind}</Tag>
                    <span className="text-caption text-ink-muted">
                      Added {relativeTime(chat.addedAt)}
                    </span>
                    {canAdmin && (
                      <Button variant="subtle" color="gray" size="xs">Remove</Button>
                    )}
                  </li>
                ))}
              </ul>
            </>
          )}
        </section>
      ))}
    </>
  )
}
