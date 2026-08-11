# cronos portal

The authoring and viewing UI. React 19 · TypeScript 7 · Vite 8 (Rolldown) ·
Tailwind CSS 4 · Mantine 9 · TanStack Router/Query/Form/Virtual · Bun · oxlint.

```bash
bun install
bun run dev      # http://localhost:5173
bun run check    # typecheck + lint + build + bundle budgets
bun run shots    # drive it in headless Chrome, write shots/
bun run test     # unit tests (SQL generation, filter compilation)
bun run verify   # every browser suite below, in one pass, against a running dev server

bun run shell    # assert the header, sidebar collapse and canvas sizing
bun run builder  # assert the report editor: canvas size, WYSIWYG, inspector
bun run identifier # assert the API-name field: collapsed, follows, stops following
bun run people   # assert user management: roles, grants, last-owner rule
bun run security # assert 2FA enrolment order, recovery codes, org policy
bun run share    # assert sharing: channels, validation, disclosure copy
bun run branding # assert logo upload: previews, print check, per-org isolation
bun run data     # assert search and paging on sources and datasets
bun run platform # assert PWA installability, the row worker, drawer nav and mobile fit
bun run icons    # regenerate PWA icons from the mark (by hand, when it changes)
bun run acl      # assert the org/project access rules in a real browser
```

## Design rules

**Every value comes from a token.** `src/theme/index.css` declares the colour,
type, radius and shadow scales inside Tailwind's `@theme`, which turns each one
into a utility: `--color-surface` gives `bg-surface`, `--text-small` gives
`text-small`. Spacing is Tailwind's default 4px step, which was already our
scale. A component that writes a raw hex or `p-[17px]` has left the system.

**Dark mode needs no `dark:` classes.** `src/theme/tokens.css` redefines the same
theme variables under `[data-theme='dark']`, so `bg-surface` resolves differently
without any component knowing. Dark is a *selected* palette validated against the
dark surface, not an inversion — adding a colour means adding both steps.

**Charts are hand-built SVG, not a charting library.** They follow the data-viz
mark spec: bars capped at 24px with a 4px rounded data-end and a 2px surface gap
between stacked segments, 2px lines, ≥8px markers with a 2px surface ring,
hairline recessive gridlines. A library would cost ~130 KB gzipped and still
need overriding to match. `ColumnChart` and `LineChart` are the reference
implementations — copy their structure.

**Series colours are indexed, never cycled.** `SERIES` in `theme.ts` is a fixed
eight-slot order chosen so adjacent pairs stay distinguishable under colour
blindness. Take slot *n* for series *n*; past eight, fold into "Other" or facet.
Never generate a ninth hue.

**Colour never carries meaning alone.** Legends accompany every chart with two or
more series, status pills always show their label, and deltas pair their colour
with an arrow.

## Two constraints that will bite

**Mantine CSS is subsetted and layered.** `src/theme/mantine.css` imports only the
component stylesheets this app renders — the full `@mantine/core/styles.css` is
~231 KB and blows the initial-route budget on its own. Adding a Mantine component
means adding its `.layer.css` there, plus any it depends on (`Select` needs
`Combobox`, `Input`, `Popover`, `ScrollArea`, `CloseButton`).

The `.layer.css` variants matter: cascade order is declared as
`theme, base, mantine, components, utilities`, so a Tailwind utility can override
a Mantine style. The unlayered stylesheets sit outside cascade layers and beat
every utility class — if a `bg-*` on a Mantine component stops working, this is
why.

Mantine's base layer — the reset, `--mantine-scale`, the colour-scheme
variables — is supplied separately by `plugins/mantine-base.ts` as
`virtual:mantine-base.css`. It is not optional: every Mantine rule is built on
`calc(… * var(--mantine-scale))`, and an undefined custom property invalidates
the whole declaration, so component CSS without the base renders controls that
look unstyled rather than throwing.

**Bundle budgets fail the build.** Initial route ≤500 KB raw and ≤150 KB gzip;
any lazy chunk ≤500 KB raw. `scripts/check-bundle-size.mjs` measures the initial
route the way a browser does — the stylesheet, entry script and every
modulepreload in `index.html`. Keep new routes behind `lazyRouteComponent`.

## Shell

Header across the top, collapsible rail beneath it. The rail collapses to 64px
of icons rather than disappearing — an icon rail is still navigable, a hidden
one is a memory test. State persists in `localStorage` and is read
synchronously on first render, so the layout does not visibly jump from wide to
narrow. `[` toggles it, ignored while typing.

**Nothing library-sized goes in the shell.** It is in the eager bundle on every
page load, so the icon set is eleven hand-drawn paths rather than an icon
package, the workspace switcher is hand-built rather than a Mantine `Menu`
(~90 KB), and the collapsed rail uses a native `title` rather than a Mantine
`Tooltip` (~30 KB). Each of those, added casually, blew the initial-route budget
on its own.

## Report editor

Palette strip · canvas · inspector, filling the viewport below a toolbar.

**There is one artifact.** No Dashboard sits alongside Report — a dashboard is a
report whose only output is interactive, and every other proposed difference is
a property rather than a type. Superset models a "report" as a schedule;
Metabase as a subscription; Sigma collapsed the distinction outright. Power BI
ships three content types and needs a genre of articles to explain them.

Two consequences in this code: a block may override the report's dataset, which
is what lets one report combine invoices and shipments; and the empty canvas
offers **templates, not types** — Dashboard, Statement, Data export — each
presetting outputs and a layout and leaving nothing behind in the model.

**The canvas is WYSIWYG in the literal sense**: blocks render through the same
`StatTile`, `ColumnChart`, `LineChart` and `DataTable` the report itself uses,
fed sample rows. Previews are `pointer-events-none`, so a click selects the
block rather than landing on a chart tooltip.

Three decisions bought the space, and each one is worth keeping:

- **Config left the block.** A card carrying its own form is twice the height of
  the thing it previews, which makes a twelve-column layout impossible to judge —
  the point of two half-width charts side by side is lost if each sits under four
  dropdowns. Settings moved to the inspector.
- **One panel, two modes.** Nothing selected → the panel is the report
  (description, folder, identifier, outputs). A block selected → it is that
  block. A second permanent rail would cost 340px to show settings nobody is
  looking at.
- **The editor collapses the app rail** via `useFocusMode()` — an override, not
  a write, so leaving restores whatever preference the person had, and the
  toggle still wins if they want it back.

Width is four visual splits (quarter / half / three-quarters / full), not a
column count. Nobody thinks "span 9"; they think "three quarters of the row".

Reorder by dragging or `⌥↑` / `⌥↓`; `Delete` removes the selection. Drag-only
reordering is unusable by keyboard.

`bun run builder` asserts the canvas is at least 950×600, that it renders real
charts and table headers rather than sketches, that the width control actually
resizes the block, and that the inspector switches modes correctly.

## Query builder

`DataSource → Dataset → Report`. A dataset is a query against one source plus
the contract it exposes; reports bind to datasets, never to a source directly.

The visual builder edits a `QueryModel` and compiles it to SQL — **one way,
never back**. Switching to SQL mode seeds the editor with the generated query
and says plainly that the builder is gone, because arbitrary SQL cannot be
turned back into steps and losing that work silently would be worse.

Two rules live in `toSql` and are covered by tests:

- **Parameters and row scope emit `{{ … }}` placeholders, never literals.** The
  engine binds them. A builder that pasted values in would be teaching the wrong
  model while quietly building an injection. `SqlView` highlights placeholders
  differently from the rest so this is visible at a glance.
- **GROUP BY is derived, never asked about.** If any column is aggregated, every
  column that is not becomes the grouping. "List every non-aggregated column in
  GROUP BY" is a rule about SQL, not about the question being asked.

Also load-bearing: an aggregate *always* gets an alias. `SUM(i.total_cents)`
with none comes back as a column called `sum`, and every field bound to it
downstream misses.

Steps are ordered the way a person thinks — what am I looking at, what else
comes with it, which columns, which rows — which is deliberately not the order
SQL is written in. Joins are offered from the schema's foreign keys, so a join
is a click rather than a formula.

## User management

Person-centric, not project-centric: the question an administrator arrives with
is "what can Dewi see?", and answering it by opening each project in turn is the
wrong shape. Each row expands to that person's project grants; the Projects tab
covers the other direction.

Three rules are enforced in the interface rather than after the fact, because a
control that offers a choice and then refuses it has already misled someone:

- **An organisation keeps at least one owner.** The last owner's role select and
  Remove button are disabled, with the reason on the control.
- **You cannot change or remove yourself.** Same treatment.
- **Org admins and owners reach every project.** Their row says "All projects ·
  via admin" rather than listing explicit grants — showing only grants would
  render them as having no access at all.

Removal confirms in place and states what is lost. A modal would hide the row
being decided about, which is the thing you want to look at.

`people.ts` and `workspace.ts` must agree on your own role in each organisation
— they are the same fact stored twice, and the first version disagreed, which
surfaced as the admin screen rendering read-only.

## Worker, PWA, mobile

**The row worker** (`workers/rows.worker.ts`) filters and aggregates. Rows are
sent **once** and kept there — posting them with every filter would swap a
main-thread cost for a structured clone of the same size, which is the usual way
a worker makes things slower. Only the filter crosses afterwards, and only a
page of rows comes back. Replies are dropped by sequence number, because a
slower earlier filter landing after a faster later one shows the wrong rows
with nothing to indicate it. There is a main-thread fallback using the *same*
function, so a worker that cannot construct degrades instead of rendering
nothing.

**PWA.** The manifest previously had `icons: []`, which is not an incomplete
manifest but an uninstallable one. `bun run icons` renders 192, 512 and a
maskable 512 from the mark and commits them, so CI never needs a browser for a
favicon. Updates **prompt** rather than auto-reload: a silent reload while
someone is mid-layout throws their work away. Definitions are cached; results
never are — they are principal-scoped, and a cache that ignores who asked is a
data leak.

**Mobile.** Every page fits 390px, asserted. The bug that caused all of it was
one line: a bare `1fr` grid column is `minmax(auto,1fr)` and refuses to shrink
below its content, so a wide child pushed the whole document sideways and took
the nav off-screen with it. Columns are `minmax(0,1fr)`; the nav strip and tab
rows scroll themselves; wide tables scroll inside their own box; header actions
wrap to their own line rather than shrinking.

## Search and paging

One search box across sources *and* datasets. Someone hunting for "invoices" is
looking for a thing, not a category, and making them guess which of two boxes
to type into is a question the interface should answer.

**Previous/next, not numbered pages.** Numbered pages commit the API to offset
paging, and an offset over a list being written to skips and repeats rows — the
"I saw that one twice on page three" bug. Previous/next reads the same to a
person and survives a later move to cursors.

Three details that are the difference between working and nearly working:

- **Searching resets to page one.** Not doing so strands you on an empty page
  three and reads as no results.
- **`paginate` clamps a page that ran off the end** rather than returning an
  empty slice, for the same reason. Unit-tested.
- **No pagination chrome when everything fits.** Controls under a list of three
  are noise answering a question nobody had.

## The cronos mark

`Brand.tsx`. An open ring with a centre pin: a lower-case **c** at size, a dial
at 16px. The name is Chronos by way of cron, and the product's distinguishing
act is delivering the same definition again and again on a schedule.

Drawn to three constraints, in order: **one colour** (it prints in black on a
statement), **16px** (the collapsed rail and the favicon are the real sizes),
and **currentColor** (light, dark and print are one asset).

Two alternates were killed by rendering them rather than by argument — a hand
pointing into the gap closed against the terminal and read as **e**; a hand to
twelve floated free and read as **¢**. The dot is r=2.2 for the same reason:
1.6 disappears at 16px, 2.8 crowds the aperture at 64.

**Lower case, always**, including at the start of a sentence — it is a wordmark,
not a word. The module path, the binary `cronosd` and every command anyone types
already spell it that way; capitalising the interface would split the brand for
no gain.

## Where each logo goes

| | Shown |
| :--- | :--- |
| cronos mark | Header and favicon |
| Organisation **mark** | Workspace switcher — the collapsed rail badge, and beside the org name |
| Organisation **wordmark** | PDF letterhead, scheduled emails, embedded views — none built yet |

The header carries the product, not the customer. cronos is multi-organisation,
and a header that changed identity on switch would confuse which product you
are in. The exception is an **embedded** view, where the customer's wordmark
replaces cronos entirely — that is white-label, and the licence gates it.

## Organisation branding

A logo here is not decoration: it lands on the paginated statement that gets
mailed to a customer, on embedded views inside someone's product, and on the
emails those arrive in. Two failure modes drive the upload, and neither shows
up in a single preview on a white card.

- **Dark ink vanishes on a dark surface.** Both surfaces are previewed side by
  side rather than one and a hope.
- **A 200px PNG is crisp in a header and a smear on paper.** Raster uploads are
  measured against print at 300dpi, not against the screen, and told what width
  they would need. A vector sidesteps the question and says so.

Two slots, because one file cannot do both jobs: a wide **wordmark** for
documents and headers, and a square **mark** for the collapsed rail and
favicons. The mark is wired into the workspace switcher, so branding is
demonstrably used rather than merely stored.

**Branding is per organisation, and lives only in the workspace context.** Two
bugs here came from the same mistake — state duplicated into a component does
not switch when the organisation does. The logo was mirrored locally and leaked
across orgs; the name and API-name fields keep genuine draft state, so that
panel is keyed by `org.id` and remounts instead.

## Sharing

A shared report has to render as *somebody*, and the whole design follows from
that. Sending produces a snapshot rendered as you; a project link sends people
through sign-in so the dataset's row-level security applies to *them*; a public
link is a frozen snapshot with an expiry.

Absent by design: a live link that runs as the sender for anyone with the URL.
That looks like a convenience and behaves like an unauthenticated export of
someone else's data — the one shape that turns row-level security into
decoration.

Every option states what the recipient will actually see, next to the control,
because "share" is the word under which data leaves.

Telegram needs its bot added to each chat first. That is an anti-spam rule in
the Bot API with no way around it, so `ChannelsPanel` explains it rather than
showing an empty recipient box.

## Two-factor authentication

Three product decisions, encoded rather than left to a settings page:

- **No SMS.** SIM-swap makes it the weakest widely-offered second factor, and
  it drags in a telephony vendor and a per-message cost to deliver it. Offering
  it lets people believe they are protected when they are not.
- **2FA is in the core, SSO is commercial.** Baseline account security behind a
  paid tier is the SSO tax with worse consequences. SSO is an enterprise
  integration; a second factor is a floor.
- **Enrolment cannot complete without verifying a code**, and recovery codes
  come last behind an explicit acknowledgement. Anything else enrols people into
  a lockout, or fills a support queue with account recovery.

The org requirement shows coverage *before* the switch — how many are protected,
and who by name would be signed out. An admin without their own second factor
cannot enable it at all: they would be the first person locked out, and then
nobody could turn it off.

## The API name field

Every creatable thing has a machine-readable name, and it was a full-weight
text input on four forms, each explaining itself differently, sitting at the
same visual weight as Name — which reads as a decision you have to make. It is
not: it is derived, and almost nobody should change it.

`IdentifierField` collapses it to one muted line and states the consequence at
the moment of editing rather than in help text read beforehand. The path prefix
appears only when opened; in a 320px rail it would otherwise eat the value,
which is the part worth checking.

**It stops following the name once edited.** Forms guard the sync with
`slugEdited` — without that, typing in Name silently destroys a deliberate edit.

## Filter builder

`FilterGroup` is the piece to preserve when things change. Boolean logic is
stated once per group as **Match all / Match any**, not as an and/or dropdown
between rows: people misread per-row joins constantly and nobody misreads "match
all of the following". `filterToText` renders the tree back as a sentence above
the panel, so a person can confirm what a report will return without reading a
single control.

Client-side evaluation in `applyFilter` exists so the mock UI behaves like the
real thing. In production the tree compiles to bound SQL predicates on the
server — the browser never decides what anyone is allowed to see.
