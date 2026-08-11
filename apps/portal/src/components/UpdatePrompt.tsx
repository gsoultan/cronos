import { useRegisterSW } from 'virtual:pwa-register/react'
import { Button } from '@mantine/core'

/**
 * Offers the new version rather than taking it.
 *
 * `autoUpdate` reloads the moment a new service worker lands, which is fine for
 * a page you read and hostile to one you work in — someone halfway through a
 * report layout loses it. This waits until they say so.
 */
export function UpdatePrompt() {
  const {
    needRefresh: [needRefresh, setNeedRefresh],
    updateServiceWorker,
  } = useRegisterSW()

  if (!needRefresh) return null

  return (
    <div role="status" data-testid="update-prompt"
      className="fixed inset-x-4 bottom-4 z-100 mx-auto flex max-w-[440px] flex-wrap
                 items-center gap-3 rounded-lg border border-line bg-surface px-4 py-3
                 shadow-pop">
      <p className="flex-1 text-small text-ink">
        A new version is ready. Anything unsaved stays until you reload.
      </p>
      <Button variant="subtle" color="gray" size="xs" onClick={() => setNeedRefresh(false)}>
        Later
      </Button>
      <Button size="xs" onClick={() => void updateServiceWorker(true)}>Reload</Button>
    </div>
  )
}
