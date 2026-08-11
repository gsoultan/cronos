// The one template. Data arrives as JSON; no Typst source is ever generated
// from a report definition — see the package doc for why that is a security
// property and not a style choice.
//
// Nothing here computes. Every number is already a string, formatted by the
// engine that knew the currency and the locale.

#let data = json("data.json")

#let page-margin = if data.page.marginMm > 0 { data.page.marginMm * 1mm } else { 18mm }

// A group's running header and footer both need to know which group the
// current page belongs to. A page can only answer that by looking backwards at
// the markers laid down in the flow, which is what these queries do.
#let group-at(p) = {
  let starts = query(<stmt-start>).filter(m => m.location().page() <= p)
  if starts.len() == 0 { none } else { starts.last() }
}

#let stmt-header = context {
  let g = group-at(here().page())
  if g != none {
    set text(size: 8pt, fill: luma(110))
    grid(
      columns: (1fr, auto),
      align(left)[*#data.org.name*],
      align(right)[#g.value — #data.title],
    )
    v(3pt)
    line(length: 100%, stroke: 0.4pt + luma(200))
  }
}

// Page X of Y, where Y is *this recipient's* statement and not the document's.
// It is the whole point of a burst: everyone receives something that reads as
// though it were produced for them alone.
#let stmt-footer = context {
  let p = here().page()
  let start = group-at(p)
  let ends = query(<stmt-end>).filter(m => m.location().page() >= p)
  if start != none and ends.len() > 0 {
    let s = start.location().page()
    let e = ends.first().location().page()
    set text(size: 8pt, fill: luma(110))
    grid(
      columns: (1fr, auto, 1fr),
      align(left)[#data.org.name],
      align(center)[Page #(p - s + 1) of #(e - s + 1)],
      align(right)[#data.period],
    )
  }
}

#set page(
  paper: if data.page.size != "" { data.page.size } else { "a4" },
  flipped: data.page.orientation == "landscape",
  margin: page-margin,
  header: stmt-header,
  footer: stmt-footer,
)

// One name, and one that ships with the typesetter. System fonts are switched
// off at the command line, so a statement is byte-identical whether it was
// typeset on a laptop that has Helvetica or in a container that has nothing —
// which is what an archived financial document requires. Brand faces arrive
// through TypstCLI.FontDir, never by happening to be installed.
#set text(font: "Libertinus Serif", size: 10pt)
#set table(stroke: none, inset: (x: 4pt, y: 5pt))

#let col-align = data.columns.map(c => if c.align == "right" { right } else { left })
#let col-width = data.columns.map(c => if c.align == "right" { auto } else { 1fr })

#let bill-to(g) = grid(
  columns: (1fr, auto),
  gutter: 12pt,
  [
    #text(size: 8pt, fill: luma(110))[BILL TO]
    #v(3pt)
    #text(weight: "semibold")[#g.label]
    #if "address" in g { for l in g.address [ \ #l ] }
  ],
  align(right)[
    #if "meta" in g {
      for kv in g.meta [
        #text(size: 8pt, fill: luma(110))[#kv.key] #h(6pt) #kv.value \
      ]
    }
  ],
)

// The subtotal row. A column carries its total if the subtotal map has its
// field; the first column carries the label instead. The template asks the
// data what a column is rather than being told which index is money.
#let subtotal-row(g) = {
  let sub = if "subtotal" in g { g.subtotal } else { (:) }
  if sub.len() == 0 { return () }
  data.columns.map(c => if c.field in sub {
    text(weight: "bold")[#sub.at(c.field)]
  } else if c.field == data.columns.first().field {
    text(weight: "bold")[Total for #g.label]
  } else { [] })
}

#for (i, g) in data.groups.enumerate() {
  if i > 0 { pagebreak() }

  [#metadata(g.label) <stmt-start>]

  block(above: 0pt, below: 14pt)[
    #text(size: 16pt, weight: "bold")[#data.title]
    #v(2pt)
    #text(size: 9pt, fill: luma(90))[#data.period]
  ]

  bill-to(g)
  v(14pt)

  table(
    columns: col-width,
    align: col-align,
    fill: (_, row) => if row == 0 { luma(242) },

    // repeat: true is the default; it is written out because it is the line
    // that stops a statement losing its column headings on page two, and a
    // silent default is a poor guard for something that matters that much.
    table.header(repeat: true, ..data.columns.map(c => text(
      size: 8pt, weight: "semibold", fill: luma(80), upper(c.label)))),

    table.hline(stroke: 0.5pt + luma(180)),
    ..g.rows.flatten(),
    table.hline(stroke: 0.5pt + luma(180)),
    ..subtotal-row(g),
  )

  [#metadata(g.label) <stmt-end>]
}
