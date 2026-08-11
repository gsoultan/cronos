/**
 * What the server said about a definition it refused.
 *
 * Shown in full. Publishing compiles every block, so the message names the
 * field — "output \"interactive\" block 0: \"profit\" is not a field of dataset
 * \"invoices\"" — and an author can act on that. Replacing it with "could not
 * save" would throw away the only part worth reading.
 */
export function PublishError({ message }: { message: string | null }) {
  if (!message) return null

  return (
    <p role="alert" data-testid="publish-error"
      className="mt-4 rounded-md border border-serious/30 bg-serious/10 px-3 py-2
                 text-small text-ink">
      <b className="font-semibold">Not saved.</b> {message}
    </p>
  )
}
