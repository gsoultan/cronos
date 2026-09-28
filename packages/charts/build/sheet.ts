/**
 * Strips the comments from the chart stylesheet as it is bundled.
 *
 * The stylesheet is a string — see src/styles.ts — so a minifier leaves every
 * comment inside it alone, and there are a lot of them, because each rule says
 * what it is for. They were most of the embed's growth from 31.7 KB to 35.8:
 * 11 KB of prose shipped to every customer's customer, read by nobody. They
 * stay in the source, where they are for.
 *
 * Only that file, and only comments: a CSS comment is the one kind of text in
 * it that nothing reads at run time.
 */
export function chartSheet() {
  return {
    name: 'cronos-chart-sheet',
    transform(code: string, id: string) {
      if (!id.replace(/\\/g, '/').endsWith('/charts/src/styles.ts')) return null
      return { code: code.replace(/\/\*[\s\S]*?\*\//g, ''), map: null }
    },
  }
}
