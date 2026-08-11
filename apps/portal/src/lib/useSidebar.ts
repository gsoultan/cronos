import { useCallback, useEffect, useState, useSyncExternalStore } from 'react'

const KEY = 'cronos.sidebar.collapsed'

/* -- Focus mode ----------------------------------------------------------- */

let focusDepth = 0
const listeners = new Set<() => void>()
const emit = () => { for (const l of listeners) l() }
const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }

/**
 * Requests a collapsed rail for as long as the calling screen is mounted.
 *
 * Editors need the width, but collapsing someone's sidebar permanently because
 * they opened a report builder would be taking a decision that is theirs. This
 * overrides the preference without writing to it, and unmounting restores
 * whatever they had. The toggle still wins: touching it drops the override, so
 * a person who wants the rail back gets it and keeps it.
 */
export function useFocusMode() {
  useEffect(() => {
    focusDepth++
    emit()
    return () => { focusDepth--; emit() }
  }, [])
}

/* -- The hook ------------------------------------------------------------- */

export function useSidebar() {
  const [collapsed, setCollapsed] = useState<boolean>(() => {
    try {
      return localStorage.getItem(KEY) === '1'
    } catch {
      return false   // private mode, or storage disabled — open is the safe default
    }
  })
  /** Set once the person touches the toggle, so focus mode stops overriding. */
  const [overridden, setOverridden] = useState(false)

  const focused = useSyncExternalStore(subscribe, () => focusDepth > 0, () => false)

  useEffect(() => {
    try {
      localStorage.setItem(KEY, collapsed ? '1' : '0')
    } catch {
      /* not worth failing a render over */
    }
  }, [collapsed])

  /* Leaving a focus screen clears the override, so the next one collapses again. */
  useEffect(() => {
    if (!focused) setOverridden(false)
  }, [focused])

  const toggle = useCallback(() => {
    setOverridden(true)
    setCollapsed((c) => !(focusDepth > 0 ? true : c))
  }, [])

  /* `[` toggles, the way most editors do it. Ignored while typing, so it never
     eats a bracket someone meant to put in a report name. */
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== '[' || e.metaKey || e.ctrlKey || e.altKey) return
      const el = document.activeElement
      const typing = el instanceof HTMLElement &&
        (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName))
      if (typing) return
      e.preventDefault()
      toggle()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [toggle])

  return { collapsed: overridden ? collapsed : collapsed || focused, toggle }
}
