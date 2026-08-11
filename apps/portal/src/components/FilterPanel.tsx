import { useState } from 'react'
import { Button, Collapse } from '@mantine/core'
import type { Field, Group } from '../lib/types'
import { FilterGroup } from './FilterGroup'
import { countConditions, filterToText } from '../lib/filterText'

interface Props {
  fields: Field[]
  value: Group
  onChange: (g: Group) => void
  onApply: () => void
}

/**
 * Filters sit in one row above the charts, collapsed to a sentence.
 *
 * The sentence is the point: a person can confirm what the report will return
 * without reading a single control, and without knowing what AND means.
 */
export function FilterPanel({ fields, value, onChange, onApply }: Props) {
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<Group>(value)

  const count = countConditions(draft)
  const sentence = filterToText(draft, fields)
  const dirty = JSON.stringify(draft) !== JSON.stringify(value)

  function apply() {
    onChange(draft)
    onApply()
    setOpen(false)
  }

  return (
    <section aria-label="Filters"
      className="mb-6 overflow-hidden rounded-lg border border-line bg-surface">
      <button type="button" data-testid="filter-toggle"
        onClick={() => setOpen((o) => !o)} aria-expanded={open}
        className="flex w-full cursor-pointer items-center gap-2 px-4 py-3 text-left
                   text-ink hover:bg-hover">
        <span aria-hidden className={`text-ink-muted transition-transform duration-150
          ease-out-quick ${open ? 'rotate-180' : ''}`}>▾</span>
        <span className="shrink-0 font-medium">
          {count === 0 ? 'Showing everything' : 'Showing where'}
        </span>
        {count > 0 && (
          <span className="truncate text-ink-secondary">{sentence}</span>
        )}
        <span className="ml-auto shrink-0 text-small font-medium text-accent">
          {count === 0 ? 'Add filter' : `${count} filter${count === 1 ? '' : 's'}`}
        </span>
      </button>

      <Collapse expanded={open}>
        <div className="border-t border-line px-4 pb-4">
          <FilterGroup group={draft} fields={fields} onChange={setDraft} />
          <div className="mt-4 flex items-center justify-between border-t border-line pt-3">
            <Button variant="subtle" color="gray" size="xs" disabled={count === 0}
              onClick={() => setDraft({ ...draft, children: [] })}>
              Clear all
            </Button>
            <div className="flex gap-2">
              <Button variant="default" size="xs"
                onClick={() => { setDraft(value); setOpen(false) }}>Cancel</Button>
              <Button size="xs" onClick={apply} disabled={!dirty}>
                {dirty ? 'Apply filters' : 'Applied'}
              </Button>
            </div>
          </div>
        </div>
      </Collapse>
    </section>
  )
}
