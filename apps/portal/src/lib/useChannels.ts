import { useMemo } from 'react'
import { useCatalog } from './useCatalog'
import { CHANNELS, type ChannelSpec } from './sharing'

/**
 * What this deployment can actually deliver through.
 *
 * One hook, because this was derived in two places and only one of them was
 * ever corrected. The share panel filtered its options by what the server
 * reported; the schedule form did not, and offered Telegram to every
 * deployment including the ones with no Telegram configured. The two paths
 * differ in when the mistake surfaces — the share panel fails while somebody
 * is watching, the schedule fails at 06:00 in the burst — so the one that was
 * left unfixed was the worse one.
 *
 * `known` are the channels this UI has a label and an icon for. `extra` are
 * channels the deployment has that this build does not know about, which are
 * still offered: a deployment that has configured something is more current
 * than a list compiled into the bundle.
 *
 * Disconnected, everything is offered. Sample mode has no server to ask, and a
 * form with an empty channel list would look broken rather than offline.
 */
export function useChannels(): {
  known: ChannelSpec[]
  extra: string[]
  options: { value: string; label: string }[]
  live: boolean
} {
  const catalog = useCatalog()
  const served = catalog.data?.channels
  const live = catalog.live

  return useMemo(() => {
    const names = served ?? []
    const known = live ? CHANNELS.filter((c) => names.includes(c.id)) : CHANNELS
    const extra = live ? names.filter((n) => !CHANNELS.some((c) => c.id === n)) : []
    return {
      known,
      extra,
      options: [
        ...known.map((c) => ({ value: c.id as string, label: c.label })),
        ...extra.map((n) => ({ value: n, label: n })),
      ],
      live,
    }
  }, [served, live])
}
