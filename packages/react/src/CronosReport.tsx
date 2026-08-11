import { useEffect, useRef, type CSSProperties } from 'react'
import { register, type CronosReport as ReportElement, type FilterValues } from '@cronos/embed'

/**
 * Teaches JSX about the custom element.
 *
 * It lives in this file rather than a .d.ts of its own because tsc does not
 * re-emit declaration files: a standalone jsx.d.ts would typecheck here and be
 * missing from the published package, so a consumer would get the error this
 * exists to prevent.
 */
declare module 'react' {
  namespace JSX {
    interface IntrinsicElements {
      /* Inline import types, not the names imported above: inside an
         augmentation of 'react' those read as private names and declaration
         emit refuses them. */
      'cronos-report': {
        ref?: import('react').Ref<ReportElement | null>
        endpoint?: string
        token?: string
        report?: string
        className?: string
        style?: import('react').CSSProperties
      }
    }
  }
}

export interface CronosReportProps {
  /** cronos base URL. */
  endpoint: string
  /** Signed embed token, minted by your server. Opaque to the browser. */
  token: string
  /** Report name. */
  report: string
  /** Narrow the report. Safe to pass an inline object — see below. */
  filters?: FilterValues
  onLoad?: (detail: { report: string }) => void
  onError?: (detail: { message: string }) => void
  className?: string
  style?: CSSProperties
}

/**
 * A cronos report in a React application.
 *
 * The element underneath is a standard custom element, so this wrapper is
 * thin by design — it exists for the three things React does not do for one:
 *
 *   1. `filters` is a *property*, not an attribute. React 18 would stringify
 *      it to "[object Object]"; React 19 assigns properties, but only for
 *      props it can see on the instance at the time it renders.
 *   2. Custom events have no React equivalent. `cronos:load` is not
 *      `onCronosLoad` in any React version, so they are bound by hand.
 *   3. Types, so `report="montly-statement"` is a compile error rather than
 *      an empty box.
 *
 * There is no `'use client'` directive and no hydration machinery. The host
 * is a client-side application, and adding server-component ceremony for a
 * case nobody has would only make this harder to read.
 */
export function CronosReport({
  endpoint, token, report, filters, onLoad, onError, className, style,
}: CronosReportProps) {
  const ref = useRef<ReportElement | null>(null)

  /* Registration is a browser-only side effect, kept out of module scope so
     importing this file is inert until something mounts. */
  useEffect(() => { register() }, [])

  /* Serialised, because a caller writing `filters={{ status: … }}` creates a
     new object on every render. Keyed on identity this would refetch forever;
     keyed on value it refetches when the value changed, which is what anyone
     writing that line meant. The element checks again for the same reason. */
  const key = JSON.stringify(filters ?? {})
  useEffect(() => {
    if (ref.current) ref.current.filters = JSON.parse(key) as FilterValues
  }, [key])

  useEffect(() => {
    const node = ref.current
    if (!node) return

    const loaded = (e: Event) => onLoad?.((e as CustomEvent).detail)
    const failed = (e: Event) => onError?.((e as CustomEvent).detail)
    node.addEventListener('cronos:load', loaded)
    node.addEventListener('cronos:error', failed)
    return () => {
      node.removeEventListener('cronos:load', loaded)
      node.removeEventListener('cronos:error', failed)
    }
  }, [onLoad, onError])

  return (
    <cronos-report
      ref={ref}
      endpoint={endpoint}
      token={token}
      report={report}
      className={className}
      style={style}
    />
  )
}
