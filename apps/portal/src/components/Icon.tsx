interface Props {
  name: IconName
  /** Decorative by default; pass a label when the icon is the only content. */
  label?: string
  className?: string
}

export type IconName =
  | 'reports' | 'data' | 'schedules' | 'activity' | 'settings'
  | 'sidebar' | 'menu' | 'search' | 'sun' | 'moon' | 'user' | 'chevron'

/*
 * A hand-drawn icon set, not a library.
 *
 * An icon package is 50–200 KB for the dozen glyphs an app this size uses, and
 * these live in the app shell — the eager bundle on every page load. Twelve
 * paths cost well under a kilobyte.
 *
 * All drawn on a 24px grid with a 1.75 stroke so they sit at the same optical
 * weight as the 13–14px text beside them.
 */
const PATHS: Record<IconName, string> = {
  reports: 'M7 3h7l5 5v13a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1Z M14 3v5h5 M9.5 13h6 M9.5 17h4',
  data: 'M4 6c0-1.1 3.6-2 8-2s8 .9 8 2-3.6 2-8 2-8-.9-8-2Z M4 6v6c0 1.1 3.6 2 8 2s8-.9 8-2V6 M4 12v6c0 1.1 3.6 2 8 2s8-.9 8-2v-6',
  schedules: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z M12 7v5l3.5 2',
  activity: 'M3 12a9 9 0 1 0 3-6.7L3 8 M3 4v4h4 M12 8v4.5l3 1.8',
  settings: 'M5 7h14 M5 12h14 M5 17h14 M9 5v4 M15 10v4 M11 15v4',
  sidebar: 'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Z M10 5v14',
  menu: 'M4 7h16 M4 12h16 M4 17h16',
  search: 'M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14Z M20 20l-4-4',
  sun: 'M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10Z M12 2v2 M12 20v2 M2 12h2 M20 12h2 M4.9 4.9l1.4 1.4 M17.7 17.7l1.4 1.4 M19.1 4.9l-1.4 1.4 M6.3 17.7l-1.4 1.4',
  moon: 'M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5Z',
  user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8Z M5 20a7 7 0 0 1 14 0',
  chevron: 'M8 10l4 4 4-4',
}

export function Icon({ name, label, className = 'size-[18px]' }: Props) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75}
      strokeLinecap="round" strokeLinejoin="round" className={`shrink-0 ${className}`}
      role={label ? 'img' : undefined} aria-label={label} aria-hidden={label ? undefined : true}>
      {PATHS[name].split(' M').map((d, i) => (
        <path key={i} d={i === 0 ? d : `M${d}`} />
      ))}
    </svg>
  )
}
