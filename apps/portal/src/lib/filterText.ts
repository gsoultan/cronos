import type { Condition, Field, FilterNode, Group } from './types'
import { specFor } from './operators'
import { shortDate } from './format'

/**
 * Renders a filter tree as a sentence. Shown above the builder so a person can
 * check what they built without reading the controls — and so a report's
 * filter can be understood at a glance from a list.
 */
export function filterToText(node: FilterNode, fields: Field[]): string {
  if (node.kind === 'condition') return conditionToText(node, fields)

  const parts = node.children.map((c) => {
    const text = filterToText(c, fields)
    return c.kind === 'group' && c.children.length > 1 ? `(${text})` : text
  }).filter(Boolean)

  if (parts.length === 0) return ''
  return parts.join(node.join === 'and' ? ' and ' : ' or ')
}

function conditionToText(c: Condition, fields: Field[]): string {
  const field = fields.find((f) => f.name === c.field)
  if (!field) return ''
  const spec = specFor(field.type, c.op)
  if (!spec) return ''

  const head = `${field.label} ${spec.label}`
  switch (spec.input) {
    case 'none':
      return head
    case 'multi': {
      const vals = Array.isArray(c.value) ? (c.value as string[]) : []
      return vals.length ? `${head} ${vals.join(', ')}` : `${head} …`
    }
    case 'numberRange':
    case 'dateRange': {
      const [a, b] = Array.isArray(c.value) ? (c.value as unknown[]) : []
      return `${head} ${fmt(a, spec.input)} and ${fmt(b, spec.input)}`
    }
    case 'relative': {
      const v = c.value as { n: number; unit: string } | undefined
      return v ? `${head} ${v.n} ${v.unit}${v.n === 1 ? '' : 's'}` : `${head} …`
    }
    default:
      return c.value === undefined || c.value === '' ? `${head} …` : `${head} ${String(c.value)}`
  }
}

function fmt(v: unknown, kind: string): string {
  if (v === undefined || v === '') return '…'
  return kind === 'dateRange' ? shortDate(String(v)) : String(v)
}

/** Conditions actually contributing — used for the "N filters" badge. */
export function countConditions(node: FilterNode): number {
  return node.kind === 'condition' ? 1 : node.children.reduce((n, c) => n + countConditions(c), 0)
}

/* -- Immutable tree edits ------------------------------------------------ */

export function updateNode(root: Group, id: string, fn: (n: FilterNode) => FilterNode): Group {
  return {
    ...root,
    children: root.children.map((c) => {
      if (c.id === id) return fn(c)
      return c.kind === 'group' ? updateNode(c, id, fn) : c
    }),
  }
}

export function removeNode(root: Group, id: string): Group {
  return {
    ...root,
    children: root.children
      .filter((c) => c.id !== id)
      .map((c) => (c.kind === 'group' ? removeNode(c, id) : c)),
  }
}

export function appendTo(root: Group, parentId: string, child: FilterNode): Group {
  if (root.id === parentId) return { ...root, children: [...root.children, child] }
  return {
    ...root,
    children: root.children.map((c) => (c.kind === 'group' ? appendTo(c, parentId, child) : c)),
  }
}

let seq = 0
export const nextId = () => `n${++seq}`
