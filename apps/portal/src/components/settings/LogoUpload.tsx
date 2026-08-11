import { useEffect, useRef, useState } from 'react'
import { Button } from '@mantine/core'

import type { Logo as LogoValue } from '../../lib/WorkspaceContext'

export type { LogoValue }

interface Props {
  label: string
  hint: string
  /** Square marks and wide wordmarks have different failure modes. */
  shape: 'wide' | 'square'
  value: LogoValue | null
  onChange: (value: LogoValue | null) => void
  /** Smallest raster width that still prints cleanly at 300dpi. */
  minPrintWidth: number
  disabled?: boolean
}

const ACCEPT = 'image/svg+xml,image/png,image/jpeg'
const MAX_BYTES = 1024 * 1024

/**
 * Uploading a logo, previewed where it will actually be used.
 *
 * Two failure modes drive this, and neither shows up in a single preview on a
 * white card. A logo drawn in dark ink vanishes on a dark surface, so both
 * surfaces are shown side by side rather than one and a hope. And a 200px PNG
 * that looks crisp in a header is a blurry smear on a printed statement — which
 * is where this logo mostly ends up — so raster uploads are measured against
 * print, not against the screen.
 */
export function LogoUpload({
  label, hint, shape, value, onChange, minPrintWidth, disabled,
}: Props) {
  const input = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)
  const [error, setError] = useState<string>()

  /* An object URL holds the file in memory until it is released. */
  useEffect(() => () => { if (value?.url.startsWith('blob:')) URL.revokeObjectURL(value.url) },
    [value?.url])

  async function accept(file: File | undefined) {
    if (!file) return
    setError(undefined)

    if (!ACCEPT.split(',').includes(file.type)) {
      setError('Use an SVG, PNG or JPEG. SVG is best — it stays sharp at any size.')
      return
    }
    if (file.size > MAX_BYTES) {
      setError(`That file is ${(file.size / 1024 / 1024).toFixed(1)} MB. Keep it under 1 MB.`)
      return
    }

    const url = URL.createObjectURL(file)
    const vector = file.type === 'image/svg+xml'
    if (vector) {
      onChange({ url, name: file.name, vector: true })
      return
    }

    const size = await measure(url)
    onChange({ url, name: file.name, vector: false, ...size })
  }

  const tooSmallForPrint = value && !value.vector && (value.width ?? 0) < minPrintWidth

  return (
    <div className="grid gap-2">
      <div>
        <span className="text-small font-semibold text-ink">{label}</span>
        <p className="mt-0.5 max-w-[62ch] text-small text-ink-secondary">{hint}</p>
      </div>

      {value ? (
        <>
          {/* Both surfaces, always. One preview cannot show the failure. */}
          <div className="grid gap-2 sm:grid-cols-2">
            {([['light', 'On light'], ['dark', 'On dark']] as const).map(([mode, caption]) => (
              <div key={mode} data-testid={`preview-${mode}`}
                className={`grid place-items-center rounded-lg border border-line p-4 ${
                  mode === 'light' ? 'bg-white' : 'bg-[#1a1a19]'}`}>
                <img src={value.url} alt={`${label} on a ${mode} background`}
                  className={shape === 'square' ? 'size-12 object-contain' : 'h-10 max-w-full object-contain'} />
                <span className={`mt-2 text-micro ${
                  mode === 'light' ? 'text-[#898781]' : 'text-[#898781]'}`}>
                  {caption}
                </span>
              </div>
            ))}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-caption text-ink-muted">
              {value.name}
              {value.vector
                ? ' · vector, prints at any size'
                : value.width ? ` · ${value.width}×${value.height}px` : ''}
            </span>
            <Button variant="default" size="xs" disabled={disabled}
              onClick={() => input.current?.click()}>Replace</Button>
            <Button variant="subtle" color="gray" size="xs" disabled={disabled}
              onClick={() => onChange(null)}>Remove</Button>
          </div>

          {tooSmallForPrint && (
            <p className="rounded-r-md border-l-2 border-serious bg-sunken px-3 py-2
                          text-small text-ink-secondary">
              <strong className="text-ink">Fine on screen, blurry on paper.</strong>{' '}
              This is {value.width}px wide; a printed statement needs about{' '}
              {minPrintWidth}px. An SVG avoids the question entirely.
            </p>
          )}
        </>
      ) : (
        <button type="button" disabled={disabled}
          data-testid={`drop-${shape}`}
          onClick={() => input.current?.click()}
          onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            e.preventDefault()
            setDragging(false)
            void accept(e.dataTransfer.files[0])
          }}
          className={`grid cursor-pointer place-items-center rounded-lg border border-dashed
            px-6 py-8 text-center disabled:cursor-default disabled:opacity-60
            ${dragging ? 'border-accent bg-accent-wash' : 'border-line bg-sunken hover:border-accent'}`}>
          <span className="text-small font-medium text-ink">
            Drop a file here, or choose one
          </span>
          <span className="mt-1 text-caption text-ink-muted">
            SVG, PNG or JPEG · up to 1 MB · {shape === 'square' ? 'square' : 'wide'}
          </span>
        </button>
      )}

      {error && (
        <p role="alert" className="flex items-baseline gap-1 text-small text-delta-bad">
          <span aria-hidden>⚠</span> {error}
        </p>
      )}

      <input ref={input} type="file" accept={ACCEPT} className="hidden"
        aria-label={label}
        onChange={(e) => { void accept(e.currentTarget.files?.[0]); e.currentTarget.value = '' }} />
    </div>
  )
}

function measure(url: string): Promise<{ width?: number; height?: number }> {
  return new Promise((resolve) => {
    const img = new Image()
    img.addEventListener('load', () => resolve({
      width: img.naturalWidth, height: img.naturalHeight,
    }), { once: true })
    // A file that will not decode simply has no dimensions to report; the
    // upload still succeeds and the print warning is skipped.
    img.addEventListener('error', () => resolve({}), { once: true })
    img.src = url
  })
}
