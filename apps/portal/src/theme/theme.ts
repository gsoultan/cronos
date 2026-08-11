import { createTheme, type MantineThemeOverride } from '@mantine/core'

/**
 * Mantine reads from the same tokens as everything else, so a Mantine control
 * and a hand-built chart cannot drift apart.
 */
export const theme: MantineThemeOverride = createTheme({
  fontFamily: 'var(--font-sans)',
  fontFamilyMonospace: 'ui-monospace, SFMono-Regular, Menlo, monospace',
  defaultRadius: 'md',
  primaryColor: 'accent',
  primaryShade: 6,
  colors: {
    // Sequential blue ramp, extended to Mantine's required 10 steps.
    accent: [
      '#eef5fe', '#cde2fb', '#9ec5f4', '#6da7ec', '#5598e7',
      '#3987e5', '#2a78d6', '#256abf', '#1c5cab', '#184f95',
    ],
  },
  radius: { sm: 'var(--radius-sm)', md: 'var(--radius-md)', lg: 'var(--radius-lg)' },
  /* Mantine's own scale, expressed in the same 4px steps Tailwind uses. */
  spacing: { xs: '8px', sm: '12px', md: '16px', lg: '24px', xl: '32px' },
  fontSizes: {
    xs: 'var(--text-caption)', sm: 'var(--text-small)', md: 'var(--text-body)',
    lg: 'var(--text-lead)', xl: 'var(--text-title)',
  },
  headings: {
    fontFamily: 'var(--font-sans)',
    sizes: {
      h1: { fontSize: 'var(--text-display)', fontWeight: '600', lineHeight: '1.2' },
      h2: { fontSize: 'var(--text-title)', fontWeight: '600', lineHeight: '1.3' },
      h3: { fontSize: 'var(--text-lead)', fontWeight: '600', lineHeight: '1.4' },
    },
  },
  components: {
    Button: { defaultProps: { size: 'sm' } },
    TextInput: { defaultProps: { size: 'sm' } },
    Select: { defaultProps: { size: 'sm', comboboxProps: { shadow: 'md' } } },
    Paper: { defaultProps: { bg: 'var(--color-surface)' } },
  },
})

/** Categorical slots, in fixed order. Index, never cycle. */
export const SERIES = [
  'var(--color-series-1)', 'var(--color-series-2)', 'var(--color-series-3)', 'var(--color-series-4)',
  'var(--color-series-5)', 'var(--color-series-6)', 'var(--color-series-7)', 'var(--color-series-8)',
] as const

/**
 * Colour follows the entity, not its rank — so a filter that removes a series
 * must not repaint the survivors. Callers pass a stable key list.
 */
export function seriesColor(key: string, allKeys: readonly string[]): string {
  const i = allKeys.indexOf(key)
  return SERIES[i] ?? 'var(--color-series-muted)'
}
