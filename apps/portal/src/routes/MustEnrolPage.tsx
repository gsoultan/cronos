import { PageHeader } from '../components/PageHeader'
import { TwoFactorSetup } from '../forms/TwoFactorSetup'
import { endSession } from '../lib/api'

/**
 * The one screen a session that may only enrol can see.
 *
 * This project requires a second factor and this account has none. Refusing the
 * sign-in would have been simpler and would lock a team out of its own
 * reporting on the afternoon somebody switched the requirement on — with no
 * self-service way back, and an administrator taking a phone call asking them
 * to turn a second factor off, which is the exact call the second factor exists
 * to make suspicious.
 *
 * So they are in, and here. The server refuses every other route to this
 * session, so this page is not the enforcement — it is what makes the
 * enforcement something a person can act on rather than a wall of 403s.
 */
export function MustEnrolPage({ onDone }: { onDone: () => void }) {
  return (
    <main className="mx-auto min-h-screen max-w-[840px] p-6">
      <PageHeader title="Set up your second factor"
        description="This project requires one. Nothing else is available until it is done — three short steps, and nothing changes until you have proved it works." />

      <TwoFactorSetup
        onDone={onDone}
        /* Not "cancel" — there is nothing to go back to. Signing out is the
           only other move, and offering it plainly is better than a button
           that returns somebody to a page they cannot use. */
        onCancel={() => void endSession()} />
    </main>
  )
}
