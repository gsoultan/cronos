import { useState } from 'react'
import { useForm, useStore } from '@tanstack/react-form'
import { Button, Select, Textarea, TextInput } from '@mantine/core'
import { Field, fieldError } from '../components/form/Field'
import { BlockPalette, PALETTE } from '../components/builder/BlockPalette'
import { LayoutCanvas } from '../components/builder/LayoutCanvas'
import { BlockInspector } from '../components/builder/BlockInspector'
import { OutputPicker } from './OutputPicker'
import { datasets } from '../lib/mock'
import type { Dataset, Tile, TileKind } from '../lib/types'
import type { Template } from '../lib/templates'
import { required, slug, toSlug } from '../lib/validators'
import { useFocusMode } from '../lib/useSidebar'

interface Props {
  onDone: () => void
  onCancel: () => void
}

let seq = 0
const nextId = () => `b${++seq}`

/**
 * The report editor: palette, canvas, inspector.
 *
 * The canvas takes the whole viewport below the toolbar because it is the work.
 * Two moves bought that space. Block configuration left the block and moved to
 * the inspector, so a card is now exactly as tall as the thing it previews —
 * which is the only way a twelve-column layout can be judged. And the report's
 * own settings share that one inspector rather than occupying a permanent rail:
 * with nothing selected the panel is the report, with a block selected it is
 * the block. One panel, never two, and the canvas keeps the rest.
 */
export function ReportForm({ onDone, onCancel }: Props) {
  /* Collapse the app rail while the editor is open — the canvas needs the
     184px more than the navigation does. The preference itself is untouched. */
  useFocusMode()

  const [blocks, setBlocks] = useState<Tile[]>([])
  const [outputs, setOutputs] = useState<string[]>(['interactive'])
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const form = useForm({
    defaultValues: { name: '', slug: '', description: '', folder: 'Finance', dataset: '' },
    onSubmit: async () => { onDone() },
  })

  /* Subscribe, do not read. `form.state` is a snapshot: reading it in the
     render body does not re-render when a field changes, so the canvas would
     never notice a dataset had been chosen. */
  const values = useStore(form.store, (s) => s.values)
  const dataset = datasets.find((d) => d.name === values.dataset)
  const fields = dataset?.fields ?? []
  const selected = blocks.find((b) => b.id === selectedId) ?? null

  /* A block reads its own dataset if it has one, otherwise the report's. */
  const datasetFor = (b: Tile): Dataset =>
    datasets.find((d) => d.name === b.dataset) ?? dataset!
  const ready = values.name.trim() !== '' && !!dataset && blocks.length > 0

  function add(kind: TileKind) {
    const preset = PALETTE.find((p) => p.kind === kind)!
    const visible = fields.filter((f) => !f.hidden)
    const block: Tile = {
      id: nextId(),
      kind,
      title: preset.label,
      span: kind === 'stat' ? 3 : kind === 'table' ? 12 : 6,
      field: visible.find((f) => f.role === 'measure')?.name,
      groupBy: kind === 'stat' ? undefined : visible.find((f) => f.role === 'dimension')?.name,
      aggregate: 'sum',
      columns: kind === 'table' ? visible.slice(0, 5).map((f) => f.name) : undefined,
    }
    setBlocks((bs) => [...bs, block])
    setSelectedId(block.id)   // new blocks open their settings, so nothing is a mystery
  }

  const patch = (p: Partial<Tile>) =>
    setBlocks((bs) => bs.map((b) => (b.id === selectedId ? { ...b, ...p } : b)))

  function applyTemplate(t: Template) {
    if (!dataset) return
    const built: Tile[] = []
    for (const b of t.build(dataset)) built.push(Object.assign({ id: nextId() }, b))
    setBlocks(built)
    setOutputs(t.outputs)
    setSelectedId(null)
  }

  return (
    <form onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); form.handleSubmit() }}
      className="flex h-[calc(100vh-7rem)] flex-col gap-3">

      {/* -- Toolbar -------------------------------------------------------- */}
      <div className="flex flex-wrap items-center gap-3 rounded-lg border border-line
                      bg-surface px-4 py-2.5 shadow-card">
        <form.Field name="name" validators={{ onBlur: ({ value }) => required('A name')(value) }}>
          {(f) => (
            <TextInput variant="unstyled" size="md" aria-label="Report name"
              placeholder="Untitled report" value={f.state.value} onBlur={f.handleBlur}
              classNames={{ input: 'font-semibold text-lead' }} className="min-w-[220px]"
              onChange={(e) => {
                f.handleChange(e.currentTarget.value)
                form.setFieldValue('slug', toSlug(e.currentTarget.value))
              }} />
          )}
        </form.Field>

        <form.Field name="dataset">
          {(f) => (
            <Select data={datasets.map((d) => ({ value: d.name, label: d.label }))}
              value={f.state.value || null} allowDeselect={false} w={200} size="sm"
              placeholder="Choose a dataset" aria-label="Dataset"
              onChange={(v) => f.handleChange(v ?? '')} />
          )}
        </form.Field>

        {dataset && (
          <span className="text-small text-ink-muted">
            {fields.filter((f) => !f.hidden).length} fields
          </span>
        )}

        <div className="ml-auto flex items-center gap-2">
          <span className="text-small text-ink-secondary max-sm:hidden">
            {ready ? `${blocks.length} block${blocks.length === 1 ? '' : 's'}`
              : 'Name it, pick a dataset, add a block'}
          </span>
          <Button variant="default" onClick={onCancel}>Cancel</Button>
          <Button type="submit" disabled={!ready}>Create report</Button>
        </div>
      </div>

      {/* -- Editor --------------------------------------------------------- */}
      <div className="flex min-h-0 flex-1 gap-3 max-lg:flex-col">
        {dataset && <BlockPalette onAdd={add} />}

        <div className="min-w-0 flex-1 overflow-auto rounded-lg border border-line bg-surface p-1">
          {dataset ? (
            <LayoutCanvas blocks={blocks} dataset={dataset} datasetFor={datasetFor}
              selectedId={selectedId} onSelect={setSelectedId} onChange={setBlocks}
              onApplyTemplate={applyTemplate} />
          ) : (
            <div className="grid h-full place-items-center px-6 text-center">
              <p className="max-w-[40ch] text-small text-ink-secondary">
                Choose a dataset in the toolbar. Blocks need to know what fields exist
                before they can show anything.
              </p>
            </div>
          )}
        </div>

        <aside data-testid="inspector"
          className="w-full shrink-0 overflow-auto rounded-lg border border-line bg-surface
                     p-4 lg:w-[320px]">
          {selected ? (
            <>
              <div className="mb-4 flex items-center justify-between gap-2">
                <h2 className="text-lead font-semibold text-ink">Block</h2>
                <button type="button" onClick={() => setSelectedId(null)}
                  className="cursor-pointer text-small text-ink-muted underline">
                  Report settings
                </button>
              </div>
              <BlockInspector block={selected} fields={datasetFor(selected).fields}
                datasets={datasets} defaultDataset={dataset!.name} onChange={patch} />
            </>
          ) : (
            <>
              <h2 className="mb-4 text-lead font-semibold text-ink">Report</h2>
              <div className="grid gap-4">
                <form.Field name="description">
                  {(f) => (
                    <Field label="Description" required={false}
                      help="Shown in the report list.">
                      <Textarea autosize minRows={2} value={f.state.value} onBlur={f.handleBlur}
                        placeholder="Per-customer statement, mailed on the 1st."
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>

                <form.Field name="folder">
                  {(f) => (
                    <Field label="Folder">
                      <Select data={['Finance', 'Operations', 'Executive']} value={f.state.value}
                        allowDeselect={false} onChange={(v) => f.handleChange(v ?? 'Finance')} />
                    </Field>
                  )}
                </form.Field>

                <form.Field name="slug" validators={{ onBlur: ({ value }) => slug(value) }}>
                  {(f) => (
                    <Field label="Identifier" error={fieldError(f.state.meta)}
                      help="Used in the API and embed URLs.">
                      <TextInput value={f.state.value} onBlur={f.handleBlur}
                        onChange={(e) => f.handleChange(e.currentTarget.value)} />
                    </Field>
                  )}
                </form.Field>

                <Field label="Outputs"
                  help="One layout, several outputs. Add more later without rebuilding.">
                  <OutputPicker value={outputs} onChange={setOutputs} compact />
                </Field>

                {blocks.length > 0 && (
                  <p className="rounded-r-md border-l-2 border-line bg-sunken px-3 py-2
                                text-caption text-ink-muted">
                    Select a block on the canvas to change what it shows.
                  </p>
                )}
              </div>
            </>
          )}
        </aside>
      </div>
    </form>
  )
}
