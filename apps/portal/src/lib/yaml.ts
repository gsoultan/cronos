/**
 * Reads and writes the small subset of YAML a definition needs.
 *
 * A library would be about thirty kilobytes to handle maps, lists, strings and
 * numbers, and this only ever handles maps, lists, strings and numbers. The set
 * of values is known: slugs, prose, whole numbers, booleans and multi-line SQL.
 *
 * Writing is safe to hand-roll because the server decodes strictly — an unknown
 * key or a mangled value is refused at publish with a message naming it, not
 * stored as a definition that means something else. That is the safety net
 * that makes this a reasonable trade rather than a brave one.
 *
 * Reading has no such net, so it is deliberately conservative: it understands
 * what this file writes plus the shapes a person hand-writes around it, and
 * anything else it does not understand comes back as a string rather than as a
 * confident guess. What protects an author from that is `unmodelled`, which
 * compares a document against what the editor would emit for it and names the
 * parts the form is not showing — so the choice to overwrite them is theirs.
 */

/** Values a definition can hold. Undefined and null are omitted entirely. */
export type Yaml =
  | string | number | boolean | null | undefined
  | Yaml[] | { [key: string]: Yaml }

/** Renders a definition document. */
export function toYaml(value: Yaml, indent = 0): string {
  if (isMap(value)) return renderMap(value, indent)
  if (Array.isArray(value)) return renderList(value, indent)
  return renderScalar(value, indent)
}

function renderMap(map: { [key: string]: Yaml }, indent: number): string {
  const pad = '  '.repeat(indent)
  const entries = Object.entries(map).filter(([, v]) => present(v))
  if (entries.length === 0) return pad + '{}'

  return entries.map(([key, value]) => {
    if (isMap(value) && Object.values(value).some(present)) {
      return `${pad}${key}:\n${renderMap(value, indent + 1)}`
    }
    if (Array.isArray(value) && value.some(present)) {
      return `${pad}${key}:\n${renderList(value, indent + 1)}`
    }
    return `${pad}${key}: ${renderScalar(value, indent)}`
  }).join('\n')
}

function renderList(list: Yaml[], indent: number): string {
  const pad = '  '.repeat(indent)
  const items = list.filter(present)
  if (items.length === 0) return pad + '[]'

  return items.map((item) => {
    if (isMap(item)) {
      // The first key sits on the dash and the rest line up under it, which is
      // what a map inside a list looks like when a person writes one.
      return pad + '- ' + renderMap(item, indent + 1).trimStart()
    }
    if (Array.isArray(item)) return `${pad}-\n${renderList(item, indent + 1)}`
    return `${pad}- ${renderScalar(item, indent)}`
  }).join('\n')
}

function renderScalar(value: Yaml, indent: number): string {
  if (!present(value)) return ''
  if (typeof value === 'string') return scalar(value, indent)
  if (Array.isArray(value)) return '[]'
  if (isMap(value)) return '{}'
  return String(value)
}

function present(v: Yaml): boolean {
  return v !== null && v !== undefined
}

function isMap(v: Yaml): v is { [key: string]: Yaml } {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

/**
 * Renders one string.
 *
 * Multi-line becomes a block scalar, because a SQL query written as a quoted
 * one-liner is a query nobody can read in the file afterwards — and reading
 * the file afterwards is the whole reason definitions are files.
 */
function scalar(s: string, indent: number): string {
  if (s.includes('\n')) {
    const pad = '  '.repeat(indent + 1)
    const body = s.replace(/\n+$/, '').split('\n').map((l) => (l ? pad + l : '')).join('\n')
    return `|\n${body}`
  }
  return needsQuotes(s) ? quote(s) : s
}

/**
 * Whether a plain scalar would be read back as something else.
 *
 * The dangerous cases are the ones that parse successfully as the wrong type:
 * `yes` is a boolean to some readers, `2026-07-01` is a timestamp, `1.0` is a
 * number, and a value beginning `{` starts a flow mapping. Quoting is cheap and
 * being wrong is a definition that means something the author did not write.
 */
function needsQuotes(s: string): boolean {
  if (s === '') return true
  if (s !== s.trim()) return true
  if (/^[-?:,[\]{}#&*!|>'"%@`]/.test(s)) return true
  if (s.includes(': ') || s.includes(' #')) return true
  if (/^(true|false|yes|no|on|off|null|~)$/i.test(s)) return true
  if (/^-?\d+(\.\d+)?([eE][-+]?\d+)?$/.test(s)) return true
  if (/^\d{4}-\d{2}-\d{2}/.test(s)) return true
  // `*` is an alias indicator, and mid-scalar it is harmless — but every cron
  // expression contains two of them and every hand-written example in this
  // repository quotes them. Matching what an author would write matters when
  // the file is the thing they read afterwards.
  if (s.includes('*')) return true
  return false
}

/** Double quotes, so an apostrophe in a description needs no thought. */
function quote(s: string): string {
  return `"${s.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}

/** Wraps a spec in the envelope every definition shares. */
export function document(kind: string, metadata: Yaml, spec: Yaml): string {
  return toYaml({ apiVersion: 'cronos.dev/v1', kind, metadata, spec }) + '\n'
}

/**
 * Reads a definition back.
 *
 * Indentation-driven, one pass, no lookahead beyond the current line. Blank
 * lines and comments are skipped; a value is a nested block if the key has
 * nothing after its colon, a block scalar if it has `|` or `>`, and a scalar
 * otherwise.
 *
 * Throws nothing. A file this cannot make sense of yields a partial tree, and
 * the caller — which is an editor deciding whether it can show a document —
 * finds out by not finding the keys it wanted, rather than by catching.
 */
export function fromYaml(source: string): Yaml {
  const lines = source.replace(/\r\n?/g, '\n').split('\n')
  let at = 0

  /** Line n, or empty past the end, so no reader has to check twice. */
  const line = (n: number): string => lines[n] ?? ''

  /** Whether line n carries nothing a parser cares about. */
  const skippable = (n: number): boolean => {
    const t = line(n).trim()
    return t === '' || t.startsWith('#') || t === '---' || t === '...'
  }
  const skip = () => { while (at < lines.length && skippable(at)) at++ }

  /** A block at column `col`: whichever of map or list starts there. */
  function block(col: number): Yaml {
    skip()
    if (at >= lines.length || indentOf(line(at)) < col) return null
    const c = indentOf(line(at))
    const t = uncomment(line(at).trim())
    return t === '-' || t.startsWith('- ') ? list(c) : map(c)
  }

  function map(col: number): { [key: string]: Yaml } {
    const out: { [key: string]: Yaml } = {}
    for (;;) {
      skip()
      if (at >= lines.length || indentOf(line(at)) !== col) break
      const text = uncomment(line(at).trim())
      if (text === '-' || text.startsWith('- ')) break

      const split = keyEnd(text)
      if (split < 0) break // not a mapping line; leave it for whoever is above
      const key = unquote(text.slice(0, split).trim())
      const rest = text.slice(split + 1).trim()
      at++

      if (rest === '') out[key] = block(col + 1)
      else if (/^[|>][-+]?$/.test(rest)) out[key] = folded(rest.slice(0, 1), col)
      else out[key] = scalarValue(rest)
    }
    return out
  }

  function list(col: number): Yaml[] {
    const out: Yaml[] = []
    for (;;) {
      skip()
      if (at >= lines.length || indentOf(line(at)) !== col) break
      const text = uncomment(line(at).trim())
      if (text !== '-' && !text.startsWith('- ')) break

      const rest = text.slice(1).trim()
      if (rest === '') { at++; out.push(block(col + 1)); continue }

      // Tested before the keyed case: `- {name: id}` is one flow mapping, and
      // its first colon belongs to the mapping rather than to the item.
      if (rest.startsWith('{') || rest.startsWith('[')) {
        at++
        out.push(scalarValue(rest))
        continue
      }
      if (keyEnd(rest) >= 0) {
        // A map whose first key sits on the dash. Re-present that key as an
        // ordinary line at the column its siblings already use, and the map
        // parser needs to know nothing about dashes.
        const after = line(at).slice(col + 1)
        const start = col + 1 + (after.length - after.trimStart().length)
        lines[at] = ' '.repeat(start) + rest
        out.push(map(start))
        continue
      }
      at++
      out.push(scalarValue(rest))
    }
    return out
  }

  /**
   * A block scalar: every following line indented past the key.
   *
   * Dedented by the first line's indent rather than by the key's, so a query
   * whose continuation lines are indented for readability keeps that shape.
   */
  function folded(style: string, col: number): string {
    const body: string[] = []
    let strip = -1
    while (at < lines.length) {
      const text = line(at)
      if (text.trim() !== '' && indentOf(text) <= col) break
      if (strip < 0 && text.trim() !== '') strip = indentOf(text)
      body.push(text.trim() === '' ? '' : text.slice(strip < 0 ? 0 : strip))
      at++
    }
    while (body.length > 0 && body[body.length - 1] === '') body.pop()
    const text = style === '>' ? body.join(' ').trim() : body.join('\n')
    return text === '' ? '' : text + '\n'
  }

  const value = block(0)
  return value
}

/** How far a line is indented. */
function indentOf(s: string): number {
  return s.length - s.trimStart().length
}

/**
 * Where the key ends, or -1 if this line is not a mapping.
 *
 * The colon must be followed by a space or end the line: `postgres://host` is
 * a scalar and `driver: postgres` is a pair, and the difference between them
 * is exactly that.
 */
function keyEnd(line: string): number {
  let inside = ''
  for (let i = 0; i < line.length; i++) {
    const ch = line[i]
    if (inside) { if (ch === inside) inside = ''; continue }
    if (ch === '"' || ch === "'") { inside = ch; continue }
    if (ch === ':' && (i + 1 === line.length || line[i + 1] === ' ')) return i
  }
  return -1
}

/** Removes a trailing comment, leaving `#` inside a quoted value alone. */
function uncomment(line: string): string {
  let inside = ''
  for (let i = 0; i < line.length; i++) {
    const ch = line[i]
    if (inside) { if (ch === inside) inside = ''; continue }
    if (ch === '"' || ch === "'") { inside = ch; continue }
    if (ch === '#' && i > 0 && line[i - 1] === ' ') return line.slice(0, i).trimEnd()
  }
  return line
}

/** One scalar, typed the way the server's decoder would type it. */
function scalarValue(s: string): Yaml {
  if (s === '' || s === '~' || s === 'null') return null
  if (s === '[]') return []
  if (s === '{}') return {}
  if (s.startsWith('"') || s.startsWith("'")) return unquote(s)
  if (s.startsWith('[') && s.endsWith(']')) return flow(s.slice(1, -1)).map(scalarValue)
  // Flow mappings, which this file never writes and every hand-written dataset
  // in the repository uses: a field list reads better as one line each than as
  // six, and refusing to read one would mean refusing to open a real file.
  if (s.startsWith('{') && s.endsWith('}')) return flowMap(s.slice(1, -1))
  if (s === 'true') return true
  if (s === 'false') return false
  // Only what round-trips: a number wide enough to lose digits is an account
  // identifier far more often than it is arithmetic, and it stays a string.
  if (/^-?\d+(\.\d+)?([eE][-+]?\d+)?$/.test(s) && String(Number(s)) === s) return Number(s)
  return s
}

/** Splits a flow collection on the commas that separate its own items. */
function flow(s: string): string[] {
  const out: string[] = []
  let inside = ''
  let depth = 0
  let start = 0
  for (let i = 0; i < s.length; i++) {
    const ch = s[i]
    if (inside) { if (ch === inside) inside = ''; continue }
    if (ch === '"' || ch === "'") { inside = ch; continue }
    if (ch === '[' || ch === '{') { depth++; continue }
    if (ch === ']' || ch === '}') { depth--; continue }
    if (ch === ',' && depth === 0) { out.push(s.slice(start, i).trim()); start = i + 1 }
  }
  const last = s.slice(start).trim()
  if (last !== '' || out.length > 0) out.push(last)
  return out.filter((v) => v !== '')
}

/** One flow mapping's pairs. */
function flowMap(s: string): { [key: string]: Yaml } {
  const out: { [key: string]: Yaml } = {}
  for (const pair of flow(s)) {
    const i = keyEnd(pair)
    if (i < 0) continue
    out[unquote(pair.slice(0, i).trim())] = scalarValue(pair.slice(i + 1).trim())
  }
  return out
}

/** Removes the quotes a writer added, and the escapes that came with them. */
function unquote(s: string): string {
  if (s.startsWith('"') && s.endsWith('"') && s.length > 1) {
    return s.slice(1, -1).replace(/\\(["\\])/g, '$1').replace(/\\n/g, '\n')
  }
  if (s.startsWith("'") && s.endsWith("'") && s.length > 1) {
    return s.slice(1, -1).replace(/''/g, "'")
  }
  return s
}

/**
 * `written`, with the keys only `stored` has folded back in.
 *
 * A form writes the keys it models and no others, so a datasource edited for
 * its name would lose its pool settings and a dataset edited for its query
 * would lose its parameters — neither of which the author touched or was shown.
 *
 * Maps merge and lists do not. A list the form wrote is a list the form owns:
 * grafting a stored `layout[1].sort` onto a reordered layout would attach a
 * sort to whichever block now sits second, which is worse than dropping it,
 * because it is wrong rather than absent. What survives a rewritten list is
 * reported by `unmodelled` instead.
 */
export function carryOver(written: Yaml, stored: Yaml): Yaml {
  if (!isMap(written) || !isMap(stored)) return written

  const out: { [key: string]: Yaml } = { ...written }
  for (const [key, value] of Object.entries(stored)) {
    if (!present(value)) continue
    if (!present(out[key])) out[key] = value
    else out[key] = carryOver(out[key], value)
  }
  return out
}

/**
 * The parts of `stored` that `rewritten` does not reproduce.
 *
 * An editor models a subset of the format. Opening a document in one and
 * saving it writes back only that subset, so a hand-written retry policy or a
 * second output profile would disappear without anybody choosing to remove it.
 * Comparing the file against what the form would emit for it turns that from a
 * silent loss into a list of paths somebody can read before they save.
 */
export function unmodelled(stored: Yaml, rewritten: Yaml, path = ''): string[] {
  if (isMap(stored)) {
    if (!isMap(rewritten)) return path ? [path] : []
    return Object.entries(stored)
      .filter(([, v]) => present(v))
      .flatMap(([k, v]) => unmodelled(v, rewritten[k], path ? `${path}.${k}` : k))
  }
  if (Array.isArray(stored)) {
    if (!Array.isArray(rewritten)) return [path]
    // Per index, so a report with a second output profile reads as "this drops
    // spec.outputs[1]" rather than as "something about spec.outputs".
    return stored.flatMap((v, i) => (i < rewritten.length
      ? unmodelled(v, rewritten[i], `${path}[${i}]`)
      : [`${path}[${i}]`]))
  }
  if (!present(stored)) return []
  // Whitespace only: a block scalar's trailing newline is not a change an
  // author made, and reporting it would bury the ones they did.
  const same = typeof stored === 'string' && typeof rewritten === 'string'
    ? stored.trim() === rewritten.trim()
    : stored === rewritten
  return same ? [] : [path]
}
