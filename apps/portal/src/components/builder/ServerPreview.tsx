import { useEffect, useState, type ReactNode } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ServerChart } from '../ServerChart'
import { connected, previewReport } from '../../lib/api'
import { draftOf } from '../../lib/definitions'
import type { Tile } from '../../lib/types'

/** How long a block is left alone before it is drawn again: long enough that
 *  typing a field name is not a query a keystroke. */
const SETTLE_MS = 600

/**
 * A block, drawn by the server as it would be published.
 *
 * This began as the map's alone — a map is its places, and made-up places
 * would be somewhere made up — while every other chart was a sketch or a
 * sample: a picture of the kind of chart, not of this one. A sketch cannot say
 * that a radar has two categories and needs three, that a box plot's measure
 * is a name, or that a stack to 100% hides the series the author meant to
 * compare; the report did, after it was published.
 *
 * So every chart and stat asks — the block as a draft, checked as publishing
 * would check it — once the author has stopped changing it for a moment, and
 * keeps the last drawing on screen while the next is made. A draft the server
 * would refuse shows the server's own sentence, which says what to fix. Until
 * the first answer, and with no server at all, the fallback stands in.
 */
export function ServerPreview({ block, dataset, fallback }: {
  block: Tile
  dataset: string
  fallback: ReactNode
}) {
  const draft = useSettled(draftOf(block, dataset), SETTLE_MS)
  const live = connected() && dataset !== ''
  const drawn = useQuery({
    // The draft is the key: the same block asked for twice is drawn once, and
    // the cache is emptied with the session, as every query here is.
    queryKey: ['preview', draft],
    queryFn: ({ signal }) => previewReport(draft, signal),
    enabled: live,
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: false,
    staleTime: 60_000,
    retry: false,
  })

  if (!live) return fallback
  if (drawn.error) return <Refused title={block.title} message={drawn.error.message} />
  const first = drawn.data?.blocks[0]
  if (!first) return fallback
  return <div data-testid="server-preview"><ServerChart block={first} /></div>
}

/** Why the server would not draw it, where the block would be. */
function Refused({ title, message }: { title: string; message: string }) {
  return (
    <div className="flex h-full flex-col overflow-hidden rounded-lg border border-line bg-surface shadow-card">
      <div className="border-b border-line px-4 py-3">
        <h3 className="text-lead font-semibold text-ink">{title}</h3>
      </div>
      <p className="px-4 py-6 text-small text-ink-secondary" data-testid="preview-refused">{message}</p>
    </div>
  )
}

/** The value, once it has stopped changing for ms. The first one at once. */
function useSettled<T>(value: T, ms: number): T {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setSettled(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return settled
}
