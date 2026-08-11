import { Button } from '@mantine/core'

interface Props {
  page: number
  pageSize: number
  total: number
  onPage: (page: number) => void
  /** Plural noun for the range readout: "sources", "datasets". */
  noun: string
}

/**
 * Previous and next, with a range readout.
 *
 * Deliberately not numbered pages. Numbered pages commit the API to offset
 * paging, and an offset over a list that is being written to skips and repeats
 * rows — the classic "I saw that one twice" on page three. Previous/next reads
 * the same to a person and survives a move to cursors later.
 *
 * Renders nothing at all when everything fits. Pagination chrome under a list
 * of three is noise that asks a question nobody had.
 */
export function Pagination({ page, pageSize, total, onPage, noun }: Props) {
  if (total <= pageSize) return null

  const first = page * pageSize + 1
  const last = Math.min(total, (page + 1) * pageSize)
  const lastPage = Math.ceil(total / pageSize) - 1

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line
                    px-4 py-3">
      <span className="text-small text-ink-secondary tabular-nums">
        {first}–{last} of {total} {noun}
      </span>
      <div className="flex gap-2">
        <Button variant="default" size="xs" disabled={page === 0}
          onClick={() => onPage(page - 1)}>Previous</Button>
        <Button variant="default" size="xs" disabled={page >= lastPage}
          onClick={() => onPage(page + 1)}>Next</Button>
      </div>
    </div>
  )
}

/** Slices a list for the current page, and clamps a page that ran off the end. */
export function paginate<T>(items: T[], page: number, pageSize: number): {
  slice: T[]
  page: number
} {
  const lastPage = Math.max(0, Math.ceil(items.length / pageSize) - 1)
  const safe = Math.min(page, lastPage)
  return { slice: items.slice(safe * pageSize, (safe + 1) * pageSize), page: safe }
}
