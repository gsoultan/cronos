import type { FieldType, Operator } from './types'

/**
 * Operators are described the way a person would say them out loud. The filter
 * builder reads as a sentence — "Issued date is in the last 30 days" — because
 * the alternative is teaching everyone SQL.
 */
export interface OperatorSpec {
  op: Operator
  /** Reads as the middle of the sentence. Lower case, no punctuation. */
  label: string
  /** What the value editor should render. */
  input: 'none' | 'text' | 'number' | 'numberRange' | 'date' | 'dateRange' | 'multi' | 'relative'
}

const TEXT_OPS: OperatorSpec[] = [
  { op: 'is', label: 'is', input: 'text' },
  { op: 'isNot', label: 'is not', input: 'text' },
  { op: 'contains', label: 'contains', input: 'text' },
  { op: 'startsWith', label: 'starts with', input: 'text' },
  { op: 'isEmpty', label: 'is empty', input: 'none' },
  { op: 'isNotEmpty', label: 'is not empty', input: 'none' },
]

const NUMBER_OPS: OperatorSpec[] = [
  { op: 'eq', label: 'equals', input: 'number' },
  { op: 'gt', label: 'is more than', input: 'number' },
  { op: 'gte', label: 'is at least', input: 'number' },
  { op: 'lt', label: 'is less than', input: 'number' },
  { op: 'lte', label: 'is at most', input: 'number' },
  { op: 'between', label: 'is between', input: 'numberRange' },
]

/**
 * Relative dates resolve against the *run* date, not the authoring date — so a
 * schedule that says "last month" keeps meaning last month.
 */
const DATE_OPS: OperatorSpec[] = [
  { op: 'inLast', label: 'is in the last', input: 'relative' },
  { op: 'thisMonth', label: 'is this month', input: 'none' },
  { op: 'lastMonth', label: 'is last month', input: 'none' },
  { op: 'thisQuarter', label: 'is this quarter', input: 'none' },
  { op: 'lastQuarter', label: 'is last quarter', input: 'none' },
  { op: 'yearToDate', label: 'is year to date', input: 'none' },
  { op: 'between', label: 'is between', input: 'dateRange' },
  { op: 'onOrAfter', label: 'is on or after', input: 'date' },
  { op: 'onOrBefore', label: 'is on or before', input: 'date' },
]

const ENUM_OPS: OperatorSpec[] = [
  { op: 'anyOf', label: 'is any of', input: 'multi' },
  { op: 'noneOf', label: 'is none of', input: 'multi' },
]

const BOOL_OPS: OperatorSpec[] = [
  { op: 'is', label: 'is', input: 'text' },
]

export function operatorsFor(type: FieldType): OperatorSpec[] {
  switch (type) {
    case 'number':
    case 'decimal': return NUMBER_OPS
    case 'date': return DATE_OPS
    case 'enum': return ENUM_OPS
    case 'bool': return BOOL_OPS
    default: return TEXT_OPS
  }
}

export function specFor(type: FieldType, op: Operator): OperatorSpec | undefined {
  return operatorsFor(type).find((s) => s.op === op)
}

/** The operator a newly added condition starts on — the most common one. */
export function defaultOperator(type: FieldType): Operator {
  switch (type) {
    case 'number':
    case 'decimal': return 'gte'
    case 'date': return 'inLast'
    case 'enum': return 'anyOf'
    default: return 'is'
  }
}

export const RELATIVE_UNITS = [
  { value: 'day', label: 'days' },
  { value: 'week', label: 'weeks' },
  { value: 'month', label: 'months' },
  { value: 'quarter', label: 'quarters' },
] as const
