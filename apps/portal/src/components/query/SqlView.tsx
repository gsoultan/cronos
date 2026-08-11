const KEYWORDS = new Set([
  'SELECT', 'FROM', 'JOIN', 'LEFT', 'INNER', 'ON', 'WHERE', 'GROUP', 'ORDER',
  'BY', 'LIMIT', 'AND', 'OR', 'NOT', 'AS', 'BETWEEN', 'IS', 'NULL', 'LIKE',
  'ANY', 'ASC', 'DESC', 'SUM', 'AVG', 'COUNT', 'MIN', 'MAX',
])

/**
 * Read-only SQL with light highlighting.
 *
 * Showing the generated query is not decoration: it is how someone checks that
 * the builder understood them, and how a SQL-literate person decides they can
 * trust it. Placeholders are highlighted separately from literals so it is
 * visible at a glance that parameters are bound rather than pasted in.
 */
export function SqlView({ sql, className = '' }: { sql: string; className?: string }) {
  return (
    <pre className={`overflow-auto rounded-md bg-sunken p-4 font-mono text-caption
                     leading-relaxed text-ink ${className}`}>
      <code>{highlight(sql)}</code>
    </pre>
  )
}

function highlight(sql: string) {
  // Split on placeholders first so their contents are never re-tokenised.
  const parts = sql.split(/(\{\{[^}]*\}\})/g)
  return parts.map((part, i) => {
    if (part.startsWith('{{')) {
      return (
        <span key={i} className="rounded-sm bg-accent-wash px-1 text-ink" title="Bound at run time">
          {part}
        </span>
      )
    }
    return part.split(/(\b\w+\b)/g).map((tok, j) => {
      if (KEYWORDS.has(tok.toUpperCase()) && /^[a-z]+$/i.test(tok)) {
        return <span key={`${i}-${j}`} className="font-semibold text-accent">{tok}</span>
      }
      if (tok.startsWith('--')) {
        return <span key={`${i}-${j}`} className="text-ink-muted">{tok}</span>
      }
      return <span key={`${i}-${j}`}>{tok}</span>
    })
  })
}
