# @cronos/embed

A report in someone else's application. One script tag, one element, **3.0 KB
gzipped** against a 40 KB budget.

```html
<script type="module" src="https://cdn.example.com/cronos-embed.js"></script>

<cronos-report
  endpoint="https://reports.acme.com"
  token="eyJhbGciOi…"
  report="monthly-invoice-statement"></cronos-report>
```

```bash
bun install
bun run size     # build and check the budget
bun run embed    # drive it in a real browser against a stub API
bun run check    # typecheck + lint + build + budget
```

## What it is not

No builder, no React, no service worker. `apps/portal` is for the people who
write reports; this is for our customers' end users, who did not ask for a
reporting product and should not pay for one in page weight.

That budget makes most of the decisions here, so they are worth stating rather
than rediscovering.

## The token is opaque

This component never decodes the token, never reads a claim from it, and never
decides what anyone may see. It carries it.

Every constraint the token pins is enforced where the query is compiled — the
one place that cannot be edited by whoever opened devtools on the host page.
Filters are therefore *sent as a request*, not applied as a fact. A filter for
a customer the token did not grant returns that caller's own rows or an error;
widening is not something the client can express.

## Values arrive formatted

Every displayable value is a string the server already formatted. Formatting
money in the browser would ship locale rules and currency data to every one of
an ISV's end users, and it would let the number in an embedded tile disagree
with the number in the PDF of the same report. The engine that knew the
currency formats it once, for both. `internal/adapter/render/paginated` makes
the same trade for the same reason.

The one exception is a bar's magnitude, which is a number because the chart has
to compare them. Its label carries the formatted text.

## No `innerHTML`, anywhere

Nodes are built and `textContent` is set. A customer name is data from *our
customer's* database — which is to say, from whoever signed up — so it is
exactly the text an attacker controls in a multi-tenant product. Building nodes
means there is no escaping to get right, rather than escaping that is currently
right. `embed-check.mjs` renders an `onerror` payload as a customer name and
asserts it stayed text.

## Shadow DOM, not an iframe

An iframe is the easy isolation, and it cannot size itself to its content,
cannot inherit a host's theme, and makes every report a scrollbar inside a
page. A shadow root gives the same style isolation with none of that: the
host's `.panel { display: none }` cannot reach in, and nothing here leaks out.

## Theming

CSS custom properties, which are the one thing that *does* cross a shadow
boundary. Set them on the element or anywhere above it.

```css
cronos-report {
  --cr-accent: #0b7285;
  --cr-surface: #fbfbf9;
  --cr-radius: 4px;
  --cr-font: 'Inter', sans-serif;
}
```

There is no `theme` attribute with a list of our opinions. Dark mode follows
`prefers-color-scheme` unless the host overrides the tokens.

Structural parts are exposed for the cases custom properties cannot reach:
`::part(panel)`, `::part(stat)`, `::part(table)`, `::part(bar)`,
`::part(grid)`, `::part(message)`, `::part(unaffected)`.

## Blocks say when a filter misses them

`docs/report-format.md` promises that a block whose dataset has no binding for a
filter says so, rather than leaving it to be discovered. The server computes
that — it is the only thing that can — and sends a `coverage` per block; this
renders "Not affected by Status" on it.

A filter that quietly applies to some blocks and not others is worse than one
that admits it: someone reading a filtered screen cannot tell which numbers
moved, and will trust the ones that did not.

## API

| | |
| :--- | :--- |
| `endpoint` | attribute — cronos base URL |
| `token` | attribute — signed embed token, opaque |
| `report` | attribute — report name |
| `filters` | **property** — `{ status: { op: 'in', values: ['overdue'] } }`; assigning reloads |
| `cronos:load` | event — a render completed |
| `cronos:error` | event — `detail.message` is safe to show a person |

Filters are a property rather than an attribute because they are structured,
and serialising them through an attribute would invite a host page to build
that string themselves.

One request is in flight at a time; a superseded one is aborted. Filters change
faster than a report loads, and a slow earlier response landing after a fast
later one shows the wrong numbers with nothing on screen saying so.
