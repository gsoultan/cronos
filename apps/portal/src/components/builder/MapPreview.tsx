import { useEffect, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ServerChart } from '../ServerChart'
import { BlockSketch } from './BlockSketch'
import { connected, previewReport } from '../../lib/api'
import { draftOf } from '../../lib/definitions'
import type { Tile } from '../../lib/types'

/** How long a map is left alone before it is drawn again: long enough that
 *  typing a field name is not a query a keystroke. */
const SETTLE_MS = 600

/**
 * A map block, drawn by the server as it would be published.
 *
 * The one block the canvas cannot draw from sample rows: a map is its places,
 * and made-up places would be somewhere made up. So it asks — the block as a
 * draft, checked as publishing would check it — once the author has stopped
 * changing it for a moment, and keeps the last map on screen while the next is
 * drawn. A draft the server would refuse shows the server's own sentence,
 * which says what to fix: a map with no places named yet says which to name.
 * With no server there is nothing to ask, and the sketch stands in.
 */
export function MapPreview({ block, dataset }: { block: Tile; dataset: string }) {
  const draft = useSettled(draftOf(block, dataset), SETTLE_MS)
  const live = connected() && dataset !== ''
  const drawn = useQuery({
    // The draft is the key: the same map asked for twice is drawn once, and
    // the cache is emptied with the session, as every query here is.
    queryKey: ['preview', draft],
    queryFn: ({ signal }) => previewReport(draft, signal),
    enabled: live,
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: false,
    staleTime: 60_000,
    retry: false,
  })

  if (!live) return <BlockSketch kind={block.kind} title={block.title} />
  if (drawn.error) return <Refused title={block.title} message={drawn.error.message} />
  const first = drawn.data?.blocks[0]
  if (!first) return <BlockSketch kind={block.kind} title={block.title} />
  return <ServerChart block={first} />
}

/** Why the server would not draw it, where the map would be. */
function Refused({ title, message }: { title: string; message: string }) {
  return (
    <div className="flex h-full flex-col overflow-hidden rounded-lg border border-line bg-surface shadow-card">
      <div className="border-b border-line px-4 py-3">
        <h3 className="text-lead font-semibold text-ink">{title}</h3>
      </div>
      <p className="px-4 py-6 text-small text-ink-secondary" data-testid="map-preview-refused">{message}</p>
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
