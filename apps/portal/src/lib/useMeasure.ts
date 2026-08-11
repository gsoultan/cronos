import { useLayoutEffect, useRef, useState } from 'react'

/**
 * Charts need real pixel width to size marks correctly — a stretched viewBox
 * distorts bar widths and stroke weights. Measure, then draw in user units.
 */
export function useMeasure<T extends HTMLElement>() {
  const ref = useRef<T | null>(null)
  const [width, setWidth] = useState(0)

  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver(([entry]) => {
      const w = entry?.contentRect.width ?? 0
      setWidth((prev) => (Math.abs(prev - w) > 0.5 ? w : prev))
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  return [ref, width] as const
}
