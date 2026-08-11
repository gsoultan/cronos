const DAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December']

/** Common shapes, offered as buttons so most people never type cron at all. */
export const CRON_PRESETS = [
  { label: 'Every weekday morning', cron: '0 7 * * 1-5' },
  { label: 'Every Monday', cron: '0 7 * * 1' },
  { label: 'First of the month', cron: '0 6 1 * *' },
  { label: 'Last day of the month', cron: '0 6 L * *' },
  { label: 'Every quarter', cron: '0 6 1 1,4,7,10 *' },
]

function ordinal(n: number): string {
  const s = ['th', 'st', 'nd', 'rd']
  const v = n % 100
  return n + (s[(v - 20) % 10] ?? s[v] ?? s[0]!)
}

function timeOf(min: string, hour: string): string {
  if (hour === '*') return min === '*' ? 'every minute' : `at ${min} minutes past every hour`
  const h = Number(hour)
  const m = Number(min)
  if (Number.isNaN(h) || Number.isNaN(m)) return `at ${hour}:${min}`
  const suffix = h < 12 ? 'am' : 'pm'
  const h12 = h % 12 === 0 ? 12 : h % 12
  return `at ${h12}${m ? `:${String(m).padStart(2, '0')}` : ''}${suffix}`
}

/**
 * Renders a cron expression as a sentence.
 *
 * The sentence is the primary control, not a decoration: almost nobody can read
 * `0 6 1 * *` with confidence, and a schedule that fires on the wrong day is
 * discovered by the recipients.
 */
export function cronToText(expr: string): string {
  const parts = expr.trim().split(/\s+/)
  if (parts.length !== 5) return 'Not a valid schedule yet'
  const [min, hour, dom, mon, dow] = parts as [string, string, string, string, string]

  const time = timeOf(min, hour)

  if (dom === '*' && dow === '*' && mon === '*') return `Every day, ${time}`

  if (dow !== '*' && dom === '*') {
    if (dow === '1-5') return `Every weekday, ${time}`
    if (dow === '0,6' || dow === '6,0') return `Every weekend day, ${time}`
    const names = dow.split(',').map((d) => DAYS[Number(d) % 7] ?? d)
    return `Every ${names.join(' and ')}, ${time}`
  }

  if (dom !== '*') {
    const day = dom.toUpperCase() === 'L' ? 'the last day' : `the ${ordinal(Number(dom))}`
    if (mon !== '*') {
      const names = mon.split(',').map((m) => MONTHS[Number(m) - 1] ?? m)
      return `On ${day} of ${names.join(', ')}, ${time}`
    }
    return `On ${day} of every month, ${time}`
  }

  return `At ${expr}`
}
