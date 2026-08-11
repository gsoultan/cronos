/* eslint-disable unicorn/require-post-message-target-origin --
   See rows.worker.ts: a second argument here is the transfer list, not an
   origin. */
import { useEffect, useRef, useState } from 'react'
import { applyFilter } from './applyFilter'
import type { Field, Group } from './types'
import type { FilterResult } from '../workers/rows.worker'

interface Options {
  rows: Record<string, unknown>[]
  fields: Field[]
  filter: Group
  sum?: string[]
  count?: { field: string; equals: string }[]
}

export interface RowState {
  total: number
  rows: Record<string, unknown>[]
  sums: Record<string, number>
  counts: Record<string, number>
  /** How long the last filter took, in the worker. */
  ms: number
  /** True while a filter is in flight — the previous result is still shown. */
  pending: boolean
  /** False when the worker could not start and the main thread is doing it. */
  offloaded: boolean
}

/**
 * Filters rows in a worker, with the main thread as a fallback.
 *
 * The fallback is not decoration: a worker can fail to construct behind a
 * restrictive CSP or in an odd embedding context, and a report that renders
 * nothing at all is worse than one that briefly janks. Same function either
 * way, so the two paths cannot disagree about what a filter means.
 *
 * Stale replies are dropped by sequence number. Filters are edited quickly and
 * a slower earlier one can land after a faster later one, which shows the wrong
 * rows with no indication anything is wrong.
 */
export function useRowWorker({ rows, fields, filter, sum, count }: Options): RowState {
  const worker = useRef<Worker | null>(null)
  const seq = useRef(0)
  const latest = useRef(0)

  const [state, setState] = useState<RowState>(() => ({
    total: rows.length, rows: rows.slice(0, 200), sums: {}, counts: {},
    ms: 0, pending: false, offloaded: false,
  }))

  useEffect(() => {
    let w: Worker
    try {
      w = new Worker(new URL('../workers/rows.worker.ts', import.meta.url), { type: 'module' })
    } catch {
      setState((s) => ({ ...s, offloaded: false }))
      return
    }
    worker.current = w
    setState((s) => ({ ...s, offloaded: true }))

    w.addEventListener('message', (e: MessageEvent<FilterResult>) => {
      const r = e.data
      if (r.id < latest.current) return   // a stale reply overtook a newer one
      latest.current = r.id
      setState({
        total: r.total, rows: r.rows, sums: r.sums, counts: r.counts,
        ms: r.ms, pending: false, offloaded: true,
      })
    })
    w.postMessage({ type: 'load', rows, fields })

    return () => { w.terminate(); worker.current = null }
  }, [rows, fields])

  useEffect(() => {
    const w = worker.current
    if (!w) {
      /* No worker: do the same work here so the page still functions. */
      const matched = applyFilter(rows, filter, fields)
      const sums: Record<string, number> = {}
      for (const f of sum ?? []) sums[f] = matched.reduce((n, r) => n + (Number(r[f]) || 0), 0)
      const counts: Record<string, number> = {}
      for (const c of count ?? []) {
        counts[`${c.field}=${c.equals}`] = matched.filter((r) => r[c.field] === c.equals).length
      }
      setState({
        total: matched.length, rows: matched.slice(0, 200), sums, counts,
        ms: 0, pending: false, offloaded: false,
      })
      return
    }

    const id = ++seq.current
    setState((s) => ({ ...s, pending: true }))
    w.postMessage({ type: 'filter', id, filter, sum, count })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filter, rows, fields, JSON.stringify(sum), JSON.stringify(count)])

  return state
}
