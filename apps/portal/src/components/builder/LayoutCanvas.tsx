import { useEffect, useState } from 'react'
import type { Field, Tile } from '../../lib/types'
import { BlockPreview } from './BlockPreview'

interface Props {
  blocks: Tile[]
  fields: Field[]
  selectedId: string | null
  onSelect: (id: string | null) => void
  onChange: (blocks: Tile[]) => void
}

/**
 * The canvas: what the report will look like, at the size it will look like it.
 *
 * Config lives in the inspector, not on the block. A card carrying its own form
 * is twice the height of the thing it is previewing, which makes a twelve-column
 * layout impossible to judge — the point of seeing two half-width charts side by
 * side is lost if each one is buried under four dropdowns.
 *
 * Blocks are selected by clicking, moved by dragging or by the keyboard, and
 * removed with Delete. The canvas background deselects, so there is always a way
 * back to the report-level settings.
 */
export function LayoutCanvas({ blocks, fields, selectedId, onSelect, onChange }: Props) {
  const [dragging, setDragging] = useState<string | null>(null)
  const [over, setOver] = useState<string | null>(null)

  function move(id: string, delta: number) {
    const i = blocks.findIndex((b) => b.id === id)
    const j = i + delta
    if (i < 0 || j < 0 || j >= blocks.length) return
    const next = [...blocks]
    const [item] = next.splice(i, 1)
    next.splice(j, 0, item!)
    onChange(next)
  }

  function drop(targetId: string) {
    if (!dragging || dragging === targetId) return
    const from = blocks.findIndex((b) => b.id === dragging)
    const to = blocks.findIndex((b) => b.id === targetId)
    if (from < 0 || to < 0) return
    const next = [...blocks]
    const [item] = next.splice(from, 1)
    next.splice(to, 0, item!)
    onChange(next)
  }

  /* Delete removes the selection; ⌥↑/⌥↓ reorder it. Ignored while typing, so
     the inspector's own inputs keep working. */
  useEffect(() => {
    if (!selectedId) return
    function onKey(e: KeyboardEvent) {
      const el = document.activeElement
      if (el instanceof HTMLElement &&
        (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName))) return

      if (e.key === 'Delete' || e.key === 'Backspace') {
        e.preventDefault()
        onChange(blocks.filter((b) => b.id !== selectedId))
        onSelect(null)
      } else if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
        e.preventDefault()
        move(selectedId!, e.key === 'ArrowUp' ? -1 : 1)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  if (blocks.length === 0) {
    return (
      <button type="button" onClick={() => onSelect(null)}
        className="grid h-full w-full cursor-default place-items-center rounded-lg border
                   border-dashed border-line bg-sunken px-6 text-center">
        <span className="max-w-[44ch]">
          <span className="block text-lead font-semibold text-ink">An empty report</span>
          <span className="mt-2 block text-small text-ink-secondary">
            Add a block from the strip on the left. Start with a number or a table —
            you can change the order and widths at any time.
          </span>
        </span>
      </button>
    )
  }

  return (
    <div data-testid="layout-canvas"
      onClick={(e) => { if (e.target === e.currentTarget) onSelect(null) }}
      onDragOver={(e) => e.preventDefault()}
      className="grid min-h-full auto-rows-min grid-cols-4 content-start gap-4 rounded-lg
                 bg-plane p-4 md:grid-cols-12">
      {blocks.map((b, i) => {
        const selected = b.id === selectedId
        return (
          <div key={b.id}
            style={{ gridColumn: `span ${b.span}` }}
            draggable
            onDragStart={() => setDragging(b.id)}
            onDragEnd={() => { setDragging(null); setOver(null) }}
            onDragEnter={() => setOver(b.id)}
            onDrop={() => { drop(b.id); setDragging(null); setOver(null) }}
            onClick={() => onSelect(b.id)}
            className={`group relative col-span-full min-w-0 rounded-lg md:[grid-column:inherit]
              ${dragging === b.id ? 'opacity-40' : ''}
              ${over === b.id && dragging !== b.id ? 'ring-2 ring-accent' : ''}
              ${selected ? 'ring-2 ring-accent' : 'hover:ring-1 hover:ring-baseline'}`}>

            {/* The preview is inert: clicks select the block rather than
                landing on a chart tooltip or a table scroller. */}
            <div className="pointer-events-none">
              <BlockPreview block={b} fields={fields} />
            </div>

            <div className={`absolute -top-3 right-2 flex items-center gap-0.5 rounded-md border
                             border-line bg-surface px-1 py-0.5 shadow-pop transition-opacity
                             ${selected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'}`}>
              <span className="cursor-grab px-1 text-ink-muted select-none"
                title="Drag to reorder" aria-hidden>⠿</span>
              <ToolButton label="Move earlier" disabled={i === 0}
                onClick={() => move(b.id, -1)}>↑</ToolButton>
              <ToolButton label="Move later" disabled={i === blocks.length - 1}
                onClick={() => move(b.id, 1)}>↓</ToolButton>
              <ToolButton label="Duplicate"
                onClick={() => {
                  const copy = { ...b, id: `${b.id}-c${blocks.length}` }
                  const next = [...blocks]
                  next.splice(i + 1, 0, copy)
                  onChange(next)
                  onSelect(copy.id)
                }}>⧉</ToolButton>
              <ToolButton label="Remove"
                onClick={() => {
                  onChange(blocks.filter((x) => x.id !== b.id))
                  if (selected) onSelect(null)
                }}>✕</ToolButton>
            </div>
          </div>
        )
      })}
    </div>
  )
}

function ToolButton({
  label, disabled, onClick, children,
}: { label: string; disabled?: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" title={label} aria-label={label} disabled={disabled}
      onClick={(e) => { e.stopPropagation(); onClick() }}
      className="grid size-6 cursor-pointer place-items-center rounded-sm text-small
                 text-ink-secondary hover:bg-hover hover:text-ink
                 disabled:cursor-default disabled:opacity-30 disabled:hover:bg-transparent">
      {children}
    </button>
  )
}
