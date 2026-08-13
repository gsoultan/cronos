import { useMemo } from 'react'
import { useCatalog } from './useCatalog'
import { datasets as sampleDatasets, reports as sampleReports } from './mock'
import type { Dataset, Field, FieldType } from './types'
import type { CatalogColumn, DatasetSummary } from './api'

/**
 * The datasets a person can build against, from the project they are in.
 *
 * The report builder and the schedule form used to import the fixture directly.
 * That fixture has two datasets with invented columns, and it was written when
 * the comment at the top of it was true — "stand-in data until the engine
 * exists". The engine exists. So a project holding five datasets offered a
 * choice of two, and choosing one of those two gave you the fixture's columns
 * rather than your own: a builder that let somebody chart `days_late` against a
 * warehouse that has no such column, and told them so only when the report
 * refused to render.
 *
 * It survived because the development script did not set VITE_CRONOS_API, so
 * sample mode was the only view anybody had — and in sample mode a fixture is
 * exactly right. Connecting the two revealed the difference immediately.
 *
 * The same shape as every other live-or-sample page here: the fixture is the
 * answer when there is no server, and never when there is one. A connected
 * portal showing invented columns beside real ones would be worse than either.
 */
export function useDatasets(): { datasets: Dataset[]; live: boolean; loading: boolean } {
  const catalog = useCatalog()

  const datasets = useMemo(() => {
    if (!catalog.live) return sampleDatasets
    return (catalog.data?.datasets ?? []).map(toDataset)
  }, [catalog.live, catalog.data])

  return { datasets, live: catalog.live, loading: catalog.isLoading }
}

/**
 * The reports a schedule can name, from the project.
 *
 * The same fault in the same place: the schedule form's report picker listed
 * the fixture's two reports, so scheduling anything you had actually written
 * meant it was not on the list. A schedule naming a report that does not exist
 * is refused at publish, which is the right refusal arriving after the wrong
 * form.
 */
export function useReportChoices(): { reports: { name: string; label: string }[]; live: boolean } {
  const catalog = useCatalog()

  const reports = useMemo(() => {
    if (!catalog.live) return sampleReports.map((r) => ({ name: r.name, label: r.label }))
    return (catalog.data?.reports ?? []).map((r) => ({ name: r.name, label: r.title || r.name }))
  }, [catalog.live, catalog.data])

  return { reports, live: catalog.live }
}

/** One dataset's summary, as the builder's model of it. */
function toDataset(d: DatasetSummary): Dataset {
  return {
    name: d.name,
    // The title where the author gave one, the name otherwise. A picker
    // showing "statement-lines" is showing an identifier; showing nothing is
    // worse.
    label: d.title || d.name,
    description: d.description,
    fields: (d.columns ?? []).map(toField),
  }
}

/**
 * A column, as a field.
 *
 * Types and roles are narrowed rather than cast: a build that learns a new
 * field type before the portal does should degrade to a string column, not
 * render a control for a type it has no idea about.
 *
 * Enum values are the one thing not carried. A dataset declares the type but
 * not the values a column actually holds — those come from the data, and the
 * report view computes them per render. So an enum here behaves as a string in
 * the builder, which is a text input rather than a wrong list of choices.
 */
function toField(c: CatalogColumn): Field {
  return {
    name: c.name,
    label: c.label || c.name,
    type: fieldType(c.type),
    role: c.role === 'measure' ? 'measure' : 'dimension',
    format: format(c.format),
    hidden: c.hidden,
  }
}

const TYPES = new Set<FieldType>(['string', 'number', 'decimal', 'date', 'bool', 'enum'])
const FORMATS = new Set(['currency', 'percent', 'number', 'preformatted'])

function fieldType(t: string): FieldType {
  return TYPES.has(t as FieldType) ? (t as FieldType) : 'string'
}

function format(f?: string): Field['format'] {
  return f && FORMATS.has(f) ? (f as Field['format']) : undefined
}
