interface MarkProps {
  className?: string
  /** Pass when the mark stands alone; omit when a wordmark names it. */
  title?: string
}

/**
 * The cronos mark: an open ring with a centre pin.
 *
 * It reads as a lower-case **c** at size and as a dial at 16px. The name is
 * Chronos by way of cron, and the product's distinguishing act is delivering
 * the same definition again and again on a schedule — so time is the honest
 * subject, and the letterform means it needs no explaining.
 *
 * Three constraints it is drawn to, in order:
 *
 * 1. **One colour.** It lands on a printed statement in black. No gradient, no
 *    two-tone, nothing that depends on a fill behind it.
 * 2. **16px.** The collapsed rail and the favicon are the real sizes. The dot
 *    is r=2.2 because 1.6 disappears there and 2.8 crowds the aperture at 64.
 * 3. **currentColor.** It inherits ink, so light, dark and print are one asset.
 *
 * Two rejected alternates, both killed by rendering them rather than by
 * argument: a hand pointing into the gap closed against the terminal and read
 * as a lower-case **e**; a hand to twelve floated free and read as **¢**.
 */
export function Mark({ className = 'size-6', title }: MarkProps) {
  return (
    <svg viewBox="0 0 32 32" fill="none" stroke="currentColor" strokeWidth={3.5}
      strokeLinecap="round" className={className}
      role={title ? 'img' : undefined} aria-label={title} aria-hidden={title ? undefined : true}>
      <path d="M22.46 7.73 A10.5 10.5 0 1 0 22.46 24.27" />
      <circle cx="16" cy="16" r="2.2" fill="currentColor" stroke="none" />
    </svg>
  )
}

interface BrandProps {
  /** Mark plus wordmark, or the mark alone. */
  wordmark?: boolean
  className?: string
}

/**
 * The lockup.
 *
 * Lower case, always — including at the start of a sentence. It is a wordmark,
 * not a word. Everything already committed spells it that way: the module path
 * `github.com/gsoultan/cronos`, the binary `cronosd`, every command anyone will
 * type. Capitalising the interface while the terminal says otherwise splits the
 * brand for no gain, and the primary buyer meets the name in a shell first.
 */
export function Brand({ wordmark = true, className = '' }: BrandProps) {
  return (
    <span className={`flex items-center gap-2 ${className}`}>
      <Mark className="size-[22px] shrink-0 text-accent"
        title={wordmark ? undefined : 'cronos'} />
      {wordmark && (
        <span className="text-lead font-semibold tracking-[-0.02em] text-ink">cronos</span>
      )}
    </span>
  )
}
