import { useMemo, useState } from 'react'
import { Button, MultiSelect, Select, Textarea, TextInput } from '@mantine/core'
import { Field } from '../form/Field'
import { ApiError, createShare } from '../../lib/api'
import { Tag } from '../StatusPill'
import {
  CHANNELS, EXPIRIES, LINK_AUDIENCES, TOKEN_HOURS, channel, invalidEmails, splitRecipients,
  telegramConnection, type Channel, type LinkAudience,
} from '../../lib/sharing'

interface Props {
  /** The report's API name, which is what a share is recorded against. */
  reportName?: string
  reportLabel: string
  projectName: string
  outputs: string[]
  onClose: () => void
}

type Tab = 'send' | 'link'

const FORMATS: Record<string, string> = {
  pdf: 'PDF — print-ready pages',
  xlsx: 'Excel — one row per record',
  csv: 'CSV — raw data',
}

/**
 * Sharing, in two shapes that are genuinely different and are not merged.
 *
 * Sending produces a snapshot rendered as you, right now. Linking either sends
 * people through sign-in — where the dataset's own row-level security applies
 * to them — or hands out a frozen snapshot to anyone holding the URL. Each
 * option states what the recipient will actually see, in a sentence, next to
 * the control, because "share" is the word under which data leaves.
 */
export function SharePanel({ reportName, reportLabel, projectName, outputs, onClose }: Props) {
  const [tab, setTab] = useState<Tab>('send')
  const [via, setVia] = useState<Channel>('email')
  const [emails, setEmails] = useState('')
  const [chats, setChats] = useState<string[]>([])
  const [format, setFormat] = useState(outputs.includes('pdf') ? 'pdf' : 'csv')
  const [note, setNote] = useState('')
  const [sent, setSent] = useState(false)

  const [audience, setAudience] = useState<LinkAudience>('project')
  const [expiry, setExpiry] = useState('7')
  const [copied, setCopied] = useState(false)
  const [link, setLink] = useState<string | null>(null)
  const [minting, setMinting] = useState(false)
  const [failed, setFailed] = useState('')

  const spec = channel(via)
  const bad = useMemo(() => (via === 'email' ? invalidEmails(emails) : []), [via, emails])
  const recipients = via === 'email' ? splitRecipients(emails).length : chats.length
  const canSend = recipients > 0 && bad.length === 0

  const chosen = LINK_AUDIENCES.find((a) => a.id === audience)!

  /* A link for people already in the project is the report's own URL: they
     sign in as themselves and the dataset's row scope applies to them. Nothing
     needs minting, and minting something would hand them a second way in that
     bypasses the first. */
  const memberUrl = `${globalThis.location?.origin ?? ''}/reports/${reportName ?? ''}`
  const url = audience === 'project' ? memberUrl : link

  /* Recorded when they ask for it, not when the panel opens. A link that
     existed because somebody looked at this tab would be one nobody chose to
     hand out and everybody would still have to revoke. */
  async function mint() {
    if (!reportName) return
    setMinting(true)
    setFailed('')
    try {
      const share = await createShare(reportName, Number(expiry))
      setLink(`${globalThis.location?.origin ?? ''}/s/${share.id}`)
    } catch (err) {
      setFailed(err instanceof ApiError ? err.message : 'Could not reach the server.')
    } finally {
      setMinting(false)
    }
  }

  /* Changing either invalidates the link already made: it was recorded with
     the old expiry, and leaving it on screen beside a new choice would be the
     panel claiming something it did not do. */
  function reconfigure(next: () => void) {
    setLink(null)
    setCopied(false)
    next()
  }

  return (
    <section className="rounded-lg border border-line bg-surface shadow-card"
      data-testid="share-panel">
      <div className="flex items-start justify-between gap-4 border-b border-line p-4">
        <div>
          <h2 className="text-lead font-semibold text-ink">Share “{reportLabel}”</h2>
          <p className="mt-1 text-small text-ink-secondary">
            To send this on a schedule instead, use Schedule.
          </p>
        </div>
        <Button variant="subtle" color="gray" size="xs" onClick={onClose}>Close</Button>
      </div>

      <div className="flex gap-1 overflow-x-auto border-b border-line px-3" role="tablist">
        {([['send', 'Send now'], ['link', 'Get a link']] as const).map(([id, label]) => (
          <button key={id} type="button" role="tab" aria-selected={tab === id}
            onClick={() => setTab(id)}
            className={`shrink-0 cursor-pointer border-b-2 px-3 py-2.5 text-small font-medium ${
              tab === id ? 'border-accent text-ink'
                : 'border-transparent text-ink-secondary hover:text-ink'}`}>
            {label}
          </button>
        ))}
      </div>

      {tab === 'send' && (
        <div className="grid max-w-[560px] gap-4 p-4">
          <Field label="Where">
            <div className="grid gap-2 sm:grid-cols-2">
              {CHANNELS.map((c) => (
                <button key={c.id} type="button" onClick={() => setVia(c.id)}
                  data-testid={`channel-${c.id}`} aria-pressed={via === c.id}
                  className={`grid cursor-pointer gap-1 rounded-lg border bg-surface p-3
                    text-left text-ink hover:border-accent
                    ${via === c.id ? 'border-accent bg-accent-wash' : 'border-line'}`}>
                  <span aria-hidden className="text-lead leading-none text-accent">{c.icon}</span>
                  <span className="font-semibold">{c.label}</span>
                  <span className="text-caption text-ink-secondary">{c.requires}</span>
                </button>
              ))}
            </div>
          </Field>

          {via === 'email' ? (
            <Field label="To"
              error={bad.length ? `Not an address: ${bad.join(', ')}` : undefined}
              help="Separate several with commas. They do not need a cronos account.">
              <Textarea autosize minRows={2} value={emails} aria-label="Email recipients"
                placeholder="dewi@acme.com, finance@acme.com"
                onChange={(e) => setEmails(e.currentTarget.value)} />
            </Field>
          ) : telegramConnection ? (
            <Field label="To"
              help={`Only chats ${telegramConnection.botUsername} has been added to can appear here.`}>
              <MultiSelect value={chats} onChange={setChats} aria-label="Telegram chats"
                placeholder="Choose a chat" searchable
                data={telegramConnection.chats.map((c) => ({
                  value: c.id, label: `${c.title} · ${c.kind}`,
                }))} />
            </Field>
          ) : (
            <p className="rounded-r-md border-l-2 border-serious bg-sunken px-4 py-3
                          text-small text-ink-secondary">
              <strong className="text-ink">Telegram is not connected.</strong> A project
              admin adds the bot in Settings → Channels. Telegram will not let a bot
              message someone who has not added it first, so there is no way around this.
            </p>
          )}

          <Field label="Send as">
            <Select allowDeselect={false} value={format} aria-label="Send as"
              onChange={(v) => setFormat(v ?? 'pdf')}
              data={outputs.filter((o) => o !== 'interactive')
                .map((o) => ({ value: o, label: FORMATS[o] ?? o }))
                .concat([{ value: 'csv', label: FORMATS.csv! }])
                .filter((o, i, a) => a.findIndex((x) => x.value === o.value) === i)} />
          </Field>

          <Field label="Message" required={false}>
            <Textarea autosize minRows={2} value={note}
              placeholder="Here is July — the overdue column is the one to look at."
              onChange={(e) => setNote(e.currentTarget.value)} />
          </Field>

          {/* The sentence that makes this honest. */}
          <p className="rounded-r-md border-l-2 border-accent bg-sunken px-4 py-3
                        text-small text-ink-secondary">
            <strong className="text-ink">This sends a snapshot of what you can see.</strong>{' '}
            It is rendered now, as you, and does not update. Recipients see your rows,
            not their own — send a link instead if they should see theirs.
            {spec.sizeLimitMb && ` ${spec.label} rejects files over ${spec.sizeLimitMb} MB.`}
          </p>

          <div className="flex items-center gap-2">
            <Button disabled={!canSend} onClick={() => setSent(true)} data-testid="send-now">
              {recipients > 1 ? `Send to ${recipients} recipients` : 'Send'}
            </Button>
            {sent && (
              <span className="text-small text-delta-good">Sent. Recorded in the audit log.</span>
            )}
          </div>
        </div>
      )}

      {tab === 'link' && (
        <div className="grid max-w-[560px] gap-4 p-4">
          <Field label="Who can open it">
            <div className="grid gap-2">
              {LINK_AUDIENCES.map((a) => (
                <button key={a.id} type="button" onClick={() => reconfigure(() => setAudience(a.id))}
                  data-testid={`audience-${a.id}`} role="radio" aria-checked={audience === a.id}
                  className={`grid cursor-pointer gap-1 rounded-lg border bg-surface p-3
                    text-left text-ink hover:border-accent
                    ${audience === a.id ? 'border-accent bg-accent-wash' : 'border-line'}`}>
                  <span className="flex items-center gap-2 font-semibold">
                    {a.label}
                    <Tag>{a.live ? 'live' : 'snapshot'}</Tag>
                  </span>
                  <span className="text-small text-ink-secondary">{a.hint}</span>
                  <span className="text-caption text-ink-muted">{a.sees}</span>
                </button>
              ))}
            </div>
          </Field>

          <Field label="Link expires"
            help={audience === 'anyone'
              ? 'Anyone holding the URL can open it until then.'
              : 'They still need to be a member of ' + projectName + '.'}>
            <Select allowDeselect={false} value={expiry} data={EXPIRIES} w={220}
              aria-label="Link expires" disabled={audience === 'project'}
              onChange={(v) => reconfigure(() => setExpiry(v ?? '7'))} />
          </Field>

          {audience === 'anyone' && expiry === '0' && (
            <p className="rounded-r-md border-l-2 border-serious bg-sunken px-4 py-3
                          text-small text-ink-secondary">
              <strong className="text-ink">A link that never expires is a password
              that never rotates.</strong> Anyone it is forwarded to keeps access until
              somebody remembers to revoke it.
            </p>
          )}

          {url ? (
            <Field label="Link">
              <div className="flex items-center gap-2">
                <TextInput readOnly value={url} className="min-w-0 flex-1" data-testid="share-link"
                  classNames={{ input: 'font-mono text-caption' }} aria-label="Share link" />
                <Button variant="default"
                  onClick={() => { void navigator.clipboard?.writeText(url); setCopied(true) }}>
                  {copied ? 'Copied' : 'Copy'}
                </Button>
              </div>
            </Field>
          ) : (
            <Button onClick={mint} loading={minting} disabled={!reportName}
              data-testid="create-link" className="justify-self-start">
              Create link
            </Button>
          )}

          {failed && (
            <p role="alert" data-testid="share-error"
              className="rounded-md border border-serious/30 bg-serious/10 px-3 py-2 text-small text-ink">
              {failed}
            </p>
          )}

          <p className="text-caption text-ink-muted">
            {chosen.sees}
            {audience === 'anyone' && (
              <> Whoever opens it gets a key to this one report that lasts {TOKEN_HOURS} hours;
              revoking the link stops it on their next request.</>
            )}
          </p>
        </div>
      )}
    </section>
  )
}
