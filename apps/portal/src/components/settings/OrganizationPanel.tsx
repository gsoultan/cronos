import { useState } from 'react'
import { Button, TextInput } from '@mantine/core'
import { Field } from '../form/Field'
import { IdentifierField } from '../form/IdentifierField'
import { LogoUpload } from './LogoUpload'
import { useWorkspace, type Branding } from '../../lib/WorkspaceContext'
import { slug as slugRule } from '../../lib/validators'

const CARD = 'mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'

/**
 * Organisation identity.
 *
 * The logo is not decoration here. It goes on the paginated statement that gets
 * mailed to a customer, on the embedded view inside somebody's product, and on
 * the emails those arrive in — so the upload states where it will appear and
 * checks it against print, which is the demanding case.
 */
export function OrganizationPanel({ canAdmin }: { canAdmin: boolean }) {
  const { org, branding, setBranding } = useWorkspace()
  const [name, setName] = useState(org.name)
  const [slug, setSlug] = useState(org.slug)
  const [slugEdited, setSlugEdited] = useState(false)

  /* Read from the context, never mirrored locally — a copy would not switch
     when the organisation does. */
  const wordmark = branding.wordmark ?? null
  const mark = branding.mark ?? null

  const apply = (next: Branding) => setBranding(org.id, { wordmark, mark, ...next })

  return (
    <>
      <section className={CARD}>
        <div className={HEAD}>
          <h2 className="text-lead font-semibold text-ink">Organization</h2>
        </div>
        <div className="grid max-w-[520px] gap-4 p-4">
          <Field label="Name" help="Shown in the workspace switcher and on anything you send.">
            <TextInput value={name} disabled={!canAdmin} aria-label="Organization name"
              onChange={(e) => {
                setName(e.currentTarget.value)
                if (!slugEdited) {
                  setSlug(e.currentTarget.value.toLowerCase().trim()
                    .replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, ''))
                }
              }} />
          </Field>
          <IdentifierField value={slug} error={slugRule(slug)}
            prefix="…/orgs/" usedFor="Every API path for this organisation contains it."
            onChange={(v) => { setSlugEdited(true); setSlug(v) }} />
        </div>
      </section>

      <section className={CARD} data-testid="branding">
        <div className={HEAD}>
          <div>
            <h2 className="text-lead font-semibold text-ink">Logo</h2>
            <p className="mt-1 max-w-[68ch] text-small text-ink-secondary">
              Appears on paginated PDFs, embedded views, scheduled emails and in
              the workspace switcher. A vector file is worth finding — the same
              logo has to work at 16px in a sidebar and at full size on a printed
              statement.
            </p>
          </div>
        </div>

        <div className="grid gap-6 p-4 lg:grid-cols-2">
          <LogoUpload label="Wordmark" shape="wide" disabled={!canAdmin}
            hint="The full logo. Used on documents, emails and the header."
            value={wordmark} minPrintWidth={600}
            onChange={(v) => apply({ wordmark: v })} />

          <LogoUpload label="Mark" shape="square" disabled={!canAdmin}
            hint="A square version for tight spaces — the collapsed sidebar, favicons, avatars."
            value={mark} minPrintWidth={256}
            onChange={(v) => apply({ mark: v })} />
        </div>

        {!canAdmin && (
          <p className="border-t border-line px-4 py-3 text-small text-ink-muted">
            Organization admins can change the logo.
          </p>
        )}

        {(wordmark || mark) && canAdmin && (
          <div className="flex flex-wrap items-center gap-3 border-t border-line bg-sunken px-4 py-3">
            <p className="flex-1 text-small text-ink-secondary">
              Changes apply to documents rendered from now on. Statements already
              sent keep the logo they were rendered with.
            </p>
            <Button size="xs">Save</Button>
          </div>
        )}
      </section>
    </>
  )
}
