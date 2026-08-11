# Paginated rendering

How cronos turns rows into a PDF, and the four decisions that shape it.

`internal/adapter/render/paginated`

## The risk, and whether it survived

`docs/product.md` names one engineering risk above the others:

> Typst layout cannot express real statement layouts — *kills the differentiator*.

It was taken early, against the hardest layout the format promises: one
statement per customer, a page break between them, column headings repeated
after every break, a subtotal per customer, and **"Page X of Y" numbering each
recipient's own statement** rather than the run's.

All of it holds. 194 rows across 7 customers, 10 pages, in 0.26 s.

The per-recipient page counter was the part in doubt, since it is the one thing
a page cannot know locally. Typst answers it by introspection: each statement
lays down `<stmt-start>` and `<stmt-end>` markers in the flow, and the footer
queries backwards and forwards from the page it is on to find the statement it
belongs to.

That mechanism is also the test oracle. `typst eval` runs a query against the
compiled document and returns JSON, so the tests assert on the layout the
template produced rather than on a PDF's glyph encoding.

**The risk is retired.** Not "looks feasible" — the layout exists and is under
test.

## Four decisions

### The template is fixed; only data varies

A report definition never becomes Typst source. Typst can read files and load
images, so generating source from definition fields would make every one of
them a potential path traversal. One embedded template with data holes has no
such surface.

The cost is real and accepted: layout variety now has to arrive as data in the
`Document` model, not as author-written Typst. A definition asking for
something the model cannot express is a change here, not a change in the
definition.

### Each render gets a private root

Typst confines every path to `--root`. Each render gets a fresh temp directory
holding exactly two files — `main.typ` and `data.json` — and that directory is
the root. Nothing else on the host is addressable, so even a template bug
cannot reach the filesystem. `TestStageWritesOnlyTheDocument` pins the
directory's contents, because the security property *is* the contents.

### System fonts are off

`--ignore-system-fonts`, always. A PDF archived today has to be reproducible
next year, and it cannot depend on what was installed on the host that rendered
it. A statement typeset on a laptop with Helvetica and one typeset in an empty
container must be the same document.

The body face is Libertinus Serif, which ships with the typesetter. Brand faces
arrive through `TypstCLI.FontDir` — never by happening to be installed.

### The burst is the memory bound

One `Document` is one PDF and is held whole; pagination is global, so there is
no streaming formulation of it. This is the one place `AGENTS.md`'s "nothing
materialises a full result set" does not apply, and the resolution is the unit
of work rather than an exception: 5,000 customers is 5,000 documents of one
customer each, not one document of 5,000. Peak memory is the largest single
recipient.

## Formatting belongs upstream

Every cell in a `Document` is already a string. The engine that knew the
currency, the locale and the rounding rule formats it; the template only places
it. A template that formats currency is a second implementation of the rules
that decided what to bill, and the two will disagree eventually.

`Document.Validate` is where a wrong document becomes a failed one. The check
that earns its place is the ragged row: a row with a missing cell does not fail
to typeset, it shifts every later cell one column left, so an amount lands
under "Status" and the statement is confidently wrong. Nothing downstream can
detect that.

## Running it

```bash
brew install typst                                  # required, not optional
go test ./internal/adapter/render/paginated/
CRONOS_PDF_OUT=/tmp/s.pdf go test ./internal/...    # leaves one to look at
```

A template is a visual artefact and no assertion sees it. Open the PDF.

## Still open

- **Deterministic bytes.** Fonts are pinned, but Typst embeds a creation date;
  two renders of the same document are not yet byte-identical. Needed before
  content-addressed run history can hash a PDF.
- **A brand sans.** Libertinus Serif is a good default and a poor brand. Ships
  with per-organisation branding, via `FontDir`.
- **The header/footer templates** the report format allows
  (`header: {template: letterhead.typ}`) are not wired up. When they are, they
  are staged into the render root like everything else — which is exactly why
  the root is per-render.
- **Compiler pooling.** A fork per render is single-digit milliseconds against
  a render of hundreds. Revisit only with a burst profile that says otherwise.
