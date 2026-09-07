# @cronos/react

A cronos report in a React application. **3.3 KB gzipped**, React not included.

```tsx
import { CronosReport } from '@cronos/react'

export function Billing({ token }: { token: string }) {
  const [status, setStatus] = useState<string | null>(null)

  return (
    <CronosReport
      endpoint="https://reports.acme.com"
      token={token}
      report="monthly-invoice-statement"
      filters={status ? { status: { op: 'eq', values: [status] } } : {}}
      onError={(e) => toast(e.message)}
    />
  )
}
```

```bash
bun run size     # build and check the budget
bun run react    # drive it in a real client-side React app
bun run check    # typecheck + lint + build + budget
```

## Where `token` comes from

From your own backend, over a route your application already authenticates.
Never from the browser, and never from a signing key in a bundle.

```tsx
// Your API route — this is the part cronos cannot do for you.
//   POST /api/reports/token  →  { token, expiresAt }
// It authenticates the user, decides their scope, and asks cronos for a token
// pinned to it. The signing key stays on your server.

function Billing() {
  const { data } = useQuery({
    queryKey: ['cronos-token'],
    queryFn: () => fetch('/api/reports/token').then((r) => r.json()),
    // Tokens last an hour by default. Refetch before they do rather than
    // after, so a long-lived tab does not show an error it could have avoided.
    refetchInterval: 45 * 60 * 1000,
  })

  if (!data) return null
  return <CronosReport endpoint="https://reports.acme.com"
    token={data.token} report="monthly-invoice-statement" />
}
```

The scope is decided **on your server, per user**. Passing a scope from the
browser would mean the browser choosing whose rows it sees, which is the whole
thing this design exists to prevent — `filters` narrows what a token already
allows and can never widen it.

Set `CRONOS_ORIGINS` to your application's exact origin, scheme included, and
never `*`. The full account is in `@cronos/embed`'s README under *Getting a
token*; everything there applies here, because this is the same element.

## Why a wrapper exists at all

The thing underneath is a standard custom element, and in Vue or Svelte you
would use it directly with no package. React needs three things it does not do
for custom elements:

1. **`filters` is a property, not an attribute.** React 18 stringifies object
   props to `"[object Object]"`. React 19 assigns properties, but only ones it
   can see on the instance when it renders.
2. **Custom events have no React equivalent.** `cronos:load` is not
   `onCronosLoad` in any React version, so listeners are bound by hand.
3. **JSX has to be told the tag exists**, or `<cronos-report>` is a type error
   and the usual fix is casting it to `any` — which throws away the checking
   this package is for.

That is the whole package. It is a thin wrapper by intent, not by accident: the
element is the API, and anything clever here would be a second API to keep in
step with it.

## An inline `filters` object is safe

```tsx
filters={{ status: { op: 'eq', values: ['overdue'] } }}
```

That is a new object on every render. Keyed on identity it would reload
forever — and each reload sets state, which renders, which builds another
object. It looks like a working demo until someone opens the network tab.

Filters are compared **by value**, both here and in the element, so this
refetches when the value changed and not when the parent re-rendered. Call
`ref.current.load()` to force a refresh.

`react-check.mjs` asserts this against a real reconciler: two parent
re-renders, zero requests.

## Client-side React is the target

There is no `'use client'` directive and no hydration machinery. Your app is a
React SPA — Vite, CRA, your own router — and adding server-component ceremony
for a case nobody has would only make this harder to read.

The bundle *is* importable on a server, so a Next.js route that imports it does
not 500. That is one guard, not a design.

## StrictMode

React's development double-mount does not double-fetch. A component that
fetches on mount without cleaning up would, and an ISV developing against us
would see doubled API usage and reasonably blame the widget.

## API

| Prop | |
| :--- | :--- |
| `endpoint` | cronos base URL |
| `token` | signed embed token, minted by your server; opaque to the browser |
| `report` | report name |
| `filters` | `{ status: { op: 'in', values: ['overdue'] } }` — compared by value |
| `onLoad` | `({ report }) => void` |
| `onError` | `({ message }) => void` — the message is safe to show a person |
| `className`, `style` | applied to the host element |

Theming is CSS custom properties on the element — see
[`@cronos/embed`](../embed/README.md). React props are for behaviour; CSS is
for appearance, and a `theme` prop would be a worse version of a stylesheet.
