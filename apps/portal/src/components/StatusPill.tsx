/**
 * Status colours are reserved and never reused as series colours, and they
 * always ship with a label — colour never carries the meaning alone.
 */
const TONE: Record<string, string> = {
  Paid: 'bg-good/15 text-delta-good',
  Delivered: 'bg-good/15 text-delta-good',
  Sent: 'bg-accent-wash text-ink',
  'In transit': 'bg-accent-wash text-ink',
  Draft: 'bg-sunken text-ink-secondary',
  Overdue: 'bg-critical/15 text-delta-bad',
  Delayed: 'bg-serious/20 text-ink',
}

export function StatusPill({ value }: { value: string }) {
  return (
    <span className={`inline-flex items-center gap-1 rounded-full px-2 py-px text-micro
                      font-medium ${TONE[value] ?? 'bg-sunken text-ink-secondary'}`}>
      {value}
    </span>
  )
}

/** A neutral label chip — outputs, roles, counts. */
export function Tag({ children, accent }: { children: React.ReactNode; accent?: boolean }) {
  return (
    <span className={`rounded-full px-2 py-px text-micro font-medium ${
      accent ? 'bg-accent-wash text-ink' : 'bg-sunken text-ink-secondary'
    }`}>
      {children}
    </span>
  )
}
