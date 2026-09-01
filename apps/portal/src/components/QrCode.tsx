import { useMemo } from 'react'
import qr from 'qrcode-generator'

/**
 * A QR code that a phone can actually read.
 *
 * What was here before drew a deterministic pattern from the secret — finder
 * squares in the corners and pseudo-random noise between them. It looked
 * exactly like a QR code and encoded nothing, so a phone pointed at it did
 * nothing at all, and the enrolment wizard behind it accepted any six digits
 * anyway. The two failures hid each other: nobody found out their authenticator
 * held no secret until the day it was asked for one.
 *
 * The encoder is a dependency rather than hand-written, which is a departure
 * for this codebase and a deliberate one. QR is Reed-Solomon over GF(256),
 * eight mask patterns scored against four penalty rules, and BCH-coded format
 * information — four hundred lines whose failure mode is a code that renders
 * beautifully and scans as nothing, which is the exact bug being removed here.
 * `qrcode-generator` is pure computation, has no dependencies of its own, and
 * touches no network.
 */
export function QrCode({ text, size = 168, label }: {
  text: string
  size?: number
  label: string
}) {
  const path = useMemo(() => {
    // Type 0 lets it pick the smallest version that fits, so a short otpauth://
    // URI is a coarse code that scans from further away. 'M' is the level every
    // authenticator app's own documentation assumes.
    const code = qr(0, 'M')
    code.addData(text)
    code.make()

    // One <path> rather than a rect per module. A version-4 code is 33×33, so
    // this is the difference between one DOM node and up to a thousand.
    const count = code.getModuleCount()
    let d = ''
    for (let row = 0; row < count; row++) {
      for (let col = 0; col < count; col++) {
        if (code.isDark(row, col)) d += `M${col} ${row}h1v1h-1z`
      }
    }
    return { d, count }
  }, [text])

  return (
    <svg viewBox={`-2 -2 ${path.count + 4} ${path.count + 4}`}
      role="img" aria-label={label} shapeRendering="crispEdges"
      style={{ width: size, height: size }}
      className="shrink-0 rounded-md bg-white p-2 ring-1 ring-line">
      {/* The quiet zone is part of the specification, not padding: a scanner
          needs the light border to find the code's edges, and a QR flush
          against a dark background often will not read. Four modules of margin
          come from the viewBox above. */}
      <rect x={-2} y={-2} width={path.count + 4} height={path.count + 4} fill="#fff" />
      <path d={path.d} fill="#0b0b0b" />
    </svg>
  )
}
