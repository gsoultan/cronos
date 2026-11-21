import type { ReportBlock } from './api'

/** One thing a reader can ask about: the whole report, or one of its blocks. */
export interface ExplainTarget {
  /** The block's place in the view, as the server counts it; absent is the report. */
  block?: number
  label: string
}

/**
 * What the "About" control offers, in the order the report draws them.
 *
 * By place rather than by title, because two blocks can share a title — a
 * "Total" stat over two datasets — and the server counts blocks the same way.
 * A block with no title is named by what it is, so the list has no blank rows.
 */
export function explainTargets(blocks: ReportBlock[]): ExplainTarget[] {
  const out: ExplainTarget[] = [{ label: 'The whole report' }]
  blocks.forEach((b, i) => {
    const what = describe(b)
    const title = b.title?.trim()
    out.push({ block: i, label: title ? `${title} — ${what}` : what })
  })
  return out
}

function describe(b: ReportBlock): string {
  switch (b.kind) {
    case 'stat': return 'figure'
    case 'table': return 'table'
    case 'text': return 'text'
    case 'chart': return b.chart === 'map' ? 'map' : `${b.chart} chart`
  }
}

/**
 * Questions somebody new to a report tends to ask, offered so the empty box is
 * not the first thing they have to solve. Phrased for whichever target is
 * chosen.
 */
export function suggestions(target: ExplainTarget): string[] {
  const it = target.block === undefined ? 'this report' : 'this'
  return [
    `What does ${it} show?`,
    'What stands out in these numbers?',
    'What do the terms here mean?',
  ]
}
