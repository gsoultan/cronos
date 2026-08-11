import { ActionIcon, Button, Select, SegmentedControl, Tooltip } from '@mantine/core'
import type { Condition, Field, FilterNode, Group } from '../lib/types'
import { defaultOperator, operatorsFor, specFor } from '../lib/operators'
import { FilterValueInput } from './FilterValueInput'
import { appendTo, nextId, removeNode, updateNode } from '../lib/filterText'

interface Props {
  group: Group
  fields: Field[]
  onChange: (g: Group) => void
  /** Root renders without the nested frame. */
  depth?: number
}

/**
 * Boolean logic, made readable.
 *
 * The join is stated once per group as "Match all / Match any" rather than an
 * and/or dropdown between every row. People reliably misread per-row joins;
 * nobody misreads "match all of the following".
 */
export function FilterGroup({ group, fields, onChange, depth = 0 }: Props) {
  const root = depth === 0

  function addCondition() {
    const field = fields[0]!
    onChange(appendTo(group, group.id, {
      id: nextId(), kind: 'condition',
      field: field.name, op: defaultOperator(field.type),
    } satisfies Condition))
  }

  function addGroup() {
    onChange(appendTo(group, group.id, {
      id: nextId(), kind: 'group', join: 'or', children: [],
    } satisfies Group))
  }

  const update = (id: string, fn: (n: FilterNode) => FilterNode) => onChange(updateNode(group, id, fn))

  return (
    <div className={root ? 'pt-4' : 'my-2 rounded-r-md border-l-2 border-accent-wash bg-sunken py-3 pl-4'}>
      <div className="flex items-center gap-3">
        <SegmentedControl size="xs" value={group.join}
          onChange={(j) => onChange({ ...group, join: j as 'and' | 'or' })}
          data={[{ label: 'Match all', value: 'and' }, { label: 'Match any', value: 'or' }]} />
        <span className="text-small text-ink-muted">
          {group.join === 'and'
            ? 'every condition below must be true'
            : 'at least one condition below must be true'}
        </span>
        {!root && (
          <Tooltip label="Remove this group" withArrow>
            <ActionIcon variant="subtle" color="gray" className="ml-auto"
              onClick={() => onChange({ ...group, children: [] })} aria-label="Remove group">
              ✕
            </ActionIcon>
          </Tooltip>
        )}
      </div>

      <div className="mt-3 grid gap-2">
        {group.children.map((child) =>
          child.kind === 'group' ? (
            <FilterGroup key={child.id} group={child} fields={fields} depth={depth + 1}
              onChange={(g) => update(child.id, () => g)} />
          ) : (
            <ConditionRow key={child.id} condition={child} fields={fields}
              onChange={(c) => update(child.id, () => c)}
              onRemove={() => onChange(removeNode(group, child.id))} />
          ),
        )}
      </div>

      <div className="mt-3 flex gap-2">
        <Button variant="subtle" size="xs" onClick={addCondition}>+ Add condition</Button>
        {depth < 2 && (
          <Tooltip label="A group lets you mix “all” and “any” together" withArrow>
            <Button variant="subtle" size="xs" color="gray" onClick={addGroup}>+ Add group</Button>
          </Tooltip>
        )}
      </div>
    </div>
  )
}

function ConditionRow({
  condition, fields, onChange, onRemove,
}: {
  condition: Condition
  fields: Field[]
  onChange: (c: Condition) => void
  onRemove: () => void
}) {
  const field = fields.find((f) => f.name === condition.field) ?? fields[0]!
  const ops = operatorsFor(field.type)
  const spec = specFor(field.type, condition.op) ?? ops[0]!

  /* Changing the field resets the operator — an operator from another type
     would be meaningless, and a silent invalid state is worse than a reset. */
  function setField(name: string | null) {
    const next = fields.find((f) => f.name === name)
    if (!next) return
    onChange({ ...condition, field: next.name, op: defaultOperator(next.type), value: undefined })
  }

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md p-2 hover:bg-hover">
      <Select data={fields.map((f) => ({ value: f.name, label: f.label }))}
        value={field.name} onChange={setField} w={190} allowDeselect={false} searchable
        aria-label="Field" />
      <Select data={ops.map((o) => ({ value: o.op, label: o.label }))}
        value={spec.op} w={170} allowDeselect={false} aria-label="Condition"
        onChange={(op) => onChange({ ...condition, op: (op ?? spec.op) as Condition['op'], value: undefined })} />
      <FilterValueInput field={field} spec={spec} value={condition.value}
        onChange={(value) => onChange({ ...condition, value })} />
      <Tooltip label="Remove" withArrow>
        <ActionIcon variant="subtle" color="gray" className="ml-auto"
          onClick={onRemove} aria-label="Remove condition">✕</ActionIcon>
      </Tooltip>
    </div>
  )
}
