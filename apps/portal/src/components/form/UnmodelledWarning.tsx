/**
 * The parts of a definition this form is not showing.
 *
 * A form models a subset of the file format. Opening a hand-written definition
 * in one and saving it writes back only that subset, so a retry policy or a
 * second output profile would disappear without anybody choosing to remove it.
 *
 * Shown rather than prevented. The author knows things this does not — that
 * the filter is obsolete, that the second profile moved elsewhere — and a form
 * that refused to save would leave them editing YAML by hand to make a change
 * the form was built for. Naming the paths is enough to make it a decision.
 */
export function UnmodelledWarning({ paths }: { paths: string[] | undefined }) {
  if (!paths || paths.length === 0) return null

  return (
    <div role="status" data-testid="unmodelled-warning"
      className="mt-4 rounded-md border border-warning/30 bg-warning/10 px-3 py-2
                 text-small text-ink">
      <b className="font-semibold">
        Saving here drops {paths.length === 1 ? 'one part' : `${paths.length} parts`} of this file.
      </b>{' '}
      This editor does not show{' '}
      {paths.map((p, i) => (
        <span key={p}>
          {i > 0 && (i === paths.length - 1 ? ' and ' : ', ')}
          <code className="font-mono text-caption">{p}</code>
        </span>
      ))}
      . Edit the file directly to keep {paths.length === 1 ? 'it' : 'them'}.
    </div>
  )
}
