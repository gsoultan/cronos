// The one template. Data arrives as JSON; no Typst source is ever generated
// from a report definition — see the package doc for why that is a security
// property and not a style choice.
//
// Nothing here computes. Every number is already a string, formatted by the
// engine that knew the currency and the locale.

#let data = json("data.json")

// A JSON null arrives as `none`, which has no `.len()` and no `.map()` — so an
// omitted list takes the whole document down rather than drawing nothing. Every
// list this template reads goes through here first, because "a key the payload
// did not carry" is a normal condition between a server and a template that
// ship separately.
#let list-of(v) = if v == none { () } else { v }

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

#let columns = list-of(data.at("columns", default: ()))
#let col-align = columns.map(c => if c.align == "right" { right } else { left })
#let col-width = columns.map(c => if c.align == "right" { auto } else { 1fr })

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
  columns.map(c => if c.field in sub {
    text(weight: "bold")[#sub.at(c.field)]
  } else if c.field == columns.first().field {
    text(weight: "bold")[Total for #g.label]
  } else { [] })
}

// -- Charts ----------------------------------------------------------------
//
// The template draws *marks*, never chart types. Every arrangement decision —
// where a bar sits, how wide a funnel band is, which vertices a pie slice has —
// is made once in Go and arrives here as rectangles, lines, polygons and dots
// in a unit box. See internal/core/document/mark.go for why: a typesetter is
// not a charting library, and a case per chart type here would be fourteen
// drawing routines kept in step with fourteen more in the browser.
//
// Nothing below computes a value. It places what it was given.

// The palette, by the names the marks use. The same hues as the viewer, so a
// PDF and the screen it was read on do not disagree about which series is
// which. Print is a light surface, so these are the light steps.
#let palette = (
  "series-1": rgb("#2a78d6"), "series-2": rgb("#eb6834"),
  "series-3": rgb("#1baf7a"), "series-4": rgb("#eda100"),
  "series-5": rgb("#e87ba4"), "series-6": rgb("#008300"),
  "series-7": rgb("#4a3aa7"), "series-8": rgb("#e34948"),
  "step-1": rgb("#86b6ef"), "step-2": rgb("#5598e7"), "step-3": rgb("#2a78d6"),
  "step-4": rgb("#1c5cab"), "step-5": rgb("#104281"),
  "ramp-1": rgb("#cde2fb"), "ramp-2": rgb("#9ec5f4"), "ramp-3": rgb("#6da7ec"),
  "ramp-4": rgb("#3987e5"), "ramp-5": rgb("#256abf"), "ramp-6": rgb("#104281"),
  "up": rgb("#2a78d6"), "down": rgb("#e34948"), "neutral": rgb("#8c8981"),
  "line": rgb("#e1e0d9"),
)

// A tone name to a colour. An unknown one is the slot-1 blue rather than an
// error: a document that fails to typeset because a future cronos sent a
// colour name is worse than one drawn in the wrong blue.
#let tone-of(name) = {
  let base = name
  let wash = name.ends-with("-wash")
  if wash { base = name.slice(0, -5) }
  let c = palette.at(base, default: palette.at("series-1"))
  if wash { c.transparentize(84%) } else { c }
}

#let chart-box = 46mm

// One mark, placed in a box of width w and height h.
#let draw-mark(m, w, h) = {
  let c = tone-of(m.at("tone", default: "series-1"))
  if m.kind == "rect" {
    place(dx: m.at("x", default: 0.0) * w, dy: m.at("y", default: 0.0) * h,
      rect(width: m.at("w", default: 0.0) * w, height: m.at("h", default: 0.0) * h,
        fill: c, stroke: none, radius: 0.6pt))
  } else if m.kind == "dot" {
    // w carries the radius for a dot, so it stays round whatever the box is.
    let r = m.at("w", default: 0.01) * w
    place(dx: m.at("x", default: 0.0) * w - r, dy: m.at("y", default: 0.0) * h - r,
      circle(radius: r, fill: c, stroke: 0.5pt + white))
  } else if m.kind == "line" {
    // `curve`, not `path`: Typst 0.15 turned `path` into the SVG-data element
    // and an array of points is no longer what it takes.
    let pts = m.points.map(p => (p.at(0) * w, p.at(1) * h))
    place(curve(stroke: 1pt + c,
      curve.move(pts.first()),
      ..pts.slice(1).map(p => curve.line(p))))
  } else if m.kind == "poly" {
    place(polygon(fill: c, stroke: none,
      ..m.points.map(p => (p.at(0) * w, p.at(1) * h))))
  }
}

// The labels beside a chart's marks.
//
// Selective, not one per mark: a label on every bar of a forty-bar chart is a
// smear, and the marks that carry a name are the ones with room for it.
#let mark-labels(marks, w, h) = {
  for m in marks {
    if m.kind == "rect" and "label" in m and m.h * h > 7pt and m.w * w > 14mm {
      place(dx: m.x * w + 2pt, dy: m.y * h + (m.h * h - 7pt) / 2,
        text(size: 6.5pt, fill: white)[#m.label])
    }
  }
}

#let chart-keys(keys) = {
  if keys.len() == 0 { return }
  v(3pt)
  grid(columns: keys.len() * (auto,), column-gutter: 8pt,
    ..keys.map(k => [
      #box(width: 5pt, height: 5pt, radius: 1pt, fill: tone-of(k.tone))
      #h(3pt)
      #text(size: 6.5pt, fill: luma(90))[#k.label]
    ]))
}

// The scale down the left of a chart that has one. The labels are the server's
// own, so a PDF and a browser round the same number the same way.
#let chart-ticks(ticks, h) = {
  box(width: 13mm, height: h, {
    for t in ticks {
      place(dy: (1.0 - t.at) * h - 4pt, dx: 0pt,
        box(width: 12mm, align(right, text(size: 6pt, fill: luma(130))[#t.label])))
    }
  })
}

#let chart(c) = block(breakable: false, above: 10pt, below: 10pt, {
  text(size: 9pt, weight: "semibold")[#c.title]
  v(4pt)
  // Defaulted, not indexed. A key absent from a newer server's payload must
  // not take the whole document down — a burst that produces no output at all
  // is worse than one chart drawn wrongly.
  let marks = list-of(c.at("marks", default: ()))
  let note = c.at("note", default: "")
  if note == none { note = "" }
  if note != "" and marks.len() == 0 {
    text(size: 8pt, fill: luma(130), style: "italic")[#note]
  } else {
    let ticks = list-of(c.at("ticks", default: ()))
    let w = if ticks.len() > 0 { 100% - 13mm } else { 100% }
    grid(columns: if ticks.len() > 0 { (13mm, 1fr) } else { (1fr,) },
      ..(if ticks.len() > 0 { (chart-ticks(ticks, chart-box),) } else { () }),
      block(width: 100%, height: chart-box, {
        // `layout`, not `measure`. Every mark is a fraction of the box, and
        // the box is a fraction of whatever width the page and its margins
        // leave — which only `layout` knows. `measure` evaluates its content in
        // isolation, where `100%` has no base to be a percentage of, so it
        // answered zero and every mark was drawn zero wide: a PDF with titles,
        // a legend, and no chart.
        layout(size => {
          // A circular chart takes a square box, centred. Scaling its points
          // by the full width would draw a pie as an ellipse, and an ellipse
          // encodes a direction the data does not have.
          let square = c.at("square", default: false) == true
          let bw = if square { chart-box } else { size.width }
          let dx = if square { (size.width - chart-box) / 2 } else { 0pt }
          place(dx: dx, {
            for m in marks { draw-mark(m, bw, chart-box) }
            mark-labels(marks, bw, chart-box)
          })
        })
      }))
    chart-keys(list-of(c.at("keys", default: ())))
    if note != "" {
      v(2pt)
      text(size: 7pt, fill: luma(110))[#note]
    }
  }
})

#for c in list-of(data.at("charts", default: ())) { chart(c) }

// A layout of charts and no table is a dashboard on paper, and a legitimate
// paginated output. The statement below is skipped rather than typeset empty.
#for (i, g) in list-of(data.at("groups", default: ())).enumerate() {
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
    table.header(repeat: true, ..columns.map(c => text(
      size: 8pt, weight: "semibold", fill: luma(80), upper(c.label)))),

    table.hline(stroke: 0.5pt + luma(180)),
    ..g.rows.flatten(),
    table.hline(stroke: 0.5pt + luma(180)),
    ..subtotal-row(g),
  )

  [#metadata(g.label) <stmt-end>]
}
