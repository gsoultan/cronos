import { useState } from 'react'
import { basemapKeys, DEFAULT_KEY, keyValueProblem, type BasemapKey } from '../../lib/maps'
import { entryFor, envName } from '../../lib/secrets'
import { useSecrets } from '../../lib/useSecrets'
import type { SecretEntry, TileMap } from '../../lib/types'
import { SecretSource } from '../settings/SecretSource'
import { SecretValueForm } from '../settings/SecretValueForm'

/**
 * Whether a map's key is set, and a place to set it.
 *
 * A Mapbox map on a server nobody gave a token draws its data on a blank
 * page, and until now the author found that out from the published report —
 * and fixed it by asking an operator for an environment variable and a
 * restart. The key goes in here instead, as the secret the map already names,
 * and the map uses it on its next render.
 *
 * Never into the file. The block's `key`, when it has one, is a reference and
 * stays one; this stores the value behind it on the server, where every
 * reader's browser is sent it with the tiles and nothing else is.
 *
 * Only against a server, and only for a basemap that has a key: OpenStreetMap
 * takes none, and on samples there is nowhere to keep one.
 */
export function BasemapKeys({ map }: { map: TileMap }) {
  const keys = basemapKeys(map)
  const { data, live, storing } = useSecrets(keys.length > 0)
  if (!live || keys.length === 0) return null

  return (
    <>
      {keys.map((k) => (
        <KeyStatus key={k.name} basemap={k} entry={entryFor(data, k.name)} storing={storing}
          shared={k.name === (map.provider ? DEFAULT_KEY[map.provider] : undefined)} />
      ))}
    </>
  )
}

function KeyStatus({ basemap, entry, storing, shared }: {
  basemap: BasemapKey
  entry: SecretEntry | undefined
  storing: boolean | undefined
  /** The provider's default, which every map in the project naming no key reads. */
  shared: boolean
}) {
  const [setting, setSetting] = useState(false)
  const [said, setSaid] = useState('')
  const { name, prefix } = basemap

  /* Refused before anything could be stored under it. The server would refuse
     the report on save and the key at render, and storing a value first would
     put a secret that was never meant for a browser one step from being sent
     to every reader. */
  if (!basemap.sendable) {
    return (
      <p role="alert" data-testid="basemap-key-refused"
        className="rounded-md border border-critical/30 bg-critical/10 px-3 py-2 text-small text-ink">
        This map’s key is <code className="font-mono text-caption">{name}</code>. A key reaches
        every reader’s browser, so the server only sends a secret named{' '}
        <code className="font-mono text-caption">{prefix}…</code> — saving refuses this one.
      </p>
    )
  }

  const stored = entry?.source === 'project'
  return (
    <div data-testid="basemap-key" data-secret={name}
      className="grid gap-2 rounded-md border border-line bg-sunken p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-small font-semibold text-ink">Key</span>
        <code className="font-mono text-caption text-ink">{name}</code>
        <SecretSource source={entry?.source} />
        {storing !== false && !setting && (
          <button type="button" data-testid="basemap-key-set"
            onClick={() => { setSetting(true); setSaid('') }}
            className="ml-auto cursor-pointer text-small text-ink-muted underline hover:text-ink">
            {stored ? 'Replace key' : 'Set key'}
          </button>
        )}
      </div>

      {shared && !setting && (
        <p className="text-caption text-ink-muted">
          Every map in this project that names no key of its own reads this one.
        </p>
      )}

      {/* Only where nothing answers yet: a key the environment already
          holds needs no advice about where to put it. */}
      {storing === false && !stored && entry?.source !== 'deployment' && (
        <p className="text-caption text-ink-secondary" data-testid="basemap-key-environment">
          This server cannot store a key: it has no CRONOS_SECRETS_KEY. Set{' '}
          <code className="font-mono">{envName(name)}</code> in its environment instead.
        </p>
      )}

      {setting && (
        <SecretValueForm name={name} label={label(prefix)} help={help(prefix)}
          submitLabel="Save key" check={(v) => keyValueProblem(name, v)}
          onCancel={() => setSetting(false)}
          onSaved={() => {
            setSetting(false)
            setSaid('Saved. The map uses it on its next render; nothing needs restarting.')
          }} />
      )}

      {said && (
        <p role="status" data-testid="basemap-key-said" className="text-caption text-ink-secondary">
          {said}
        </p>
      )}
    </div>
  )
}

/** What the value is called, by whoever issues it. */
function label(prefix: string): string {
  if (prefix === 'mapbox-') return 'Mapbox public token'
  if (prefix === 'google-') return 'Google Maps API key'
  return 'Tile key'
}

/** Said before it is typed: every one of these is sent to every reader. */
function help(prefix: string): string {
  if (prefix === 'mapbox-') {
    return 'A pk. token with the styles:tiles scope. Every reader’s browser receives it, so '
      + 'restrict it to your sites in Mapbox’s account page.'
  }
  if (prefix === 'google-') {
    return 'Every reader’s browser receives it. Restrict it to the Map Tiles API and to your '
      + 'sites in the Google Cloud console.'
  }
  return 'Sent in every tile request, so every reader’s browser receives it.'
}
