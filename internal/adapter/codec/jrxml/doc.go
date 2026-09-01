/*
Package jrxml reads JasperReports definitions and writes cronos ones.

A migration tool, and the only place in the codebase that reads a format cronos
does not own. It exists because the switching cost of a reporting estate is the
estate: four hundred `.jrxml` files hold years of decisions about which join is
correct, and asking a team to retype them is asking them not to move.

# What it can and cannot be

The two formats disagree about what a report is, and no amount of care makes
the disagreement go away.

JasperReports is a band-based pixel layout: elements sit at (x, y) inside a
title, detail or group band, and the document is what those coordinates draw.
cronos is semantic: a table block groups and subtotals, and the renderer decides
where that lands on paper. There is no function from the first to the second,
because the first does not record what a column *is* — only where it was.

So this package imports meaning and abandons appearance. It reads the query,
the parameters, the fields, the groups, the subtotals and the page setup, and
it infers a table from the fact that a detail band's text fields in reading
order are, in every tabular Jasper report ever written, the columns. It does not
carry fonts, colours, borders, pixel positions or Java expressions.

# Why it reports rather than warns

Silence is the failure mode that matters here. An importer that quietly drops a
subreport produces a definition that loads, runs, renders, and is missing a
third of the statement — and the person who finds out is the customer holding
the PDF. Everything not carried is a [Finding] against the file that caused it,
graded so an estate can be triaged: [Blocked] needs a person, [Review] needs a
look, [Note] is the appearance nobody expected to survive.

That is also why [Import] returns a [Result] with findings *and* definitions
rather than an error. A file with fourteen cosmetic losses and a working query
is a successful import; refusing it would mean the tool only accepts files that
did not need it.

# What is refused outright

Two constructs are errors rather than findings, because importing them would
produce a definition that is wrong rather than incomplete:

  - `$P!{x}`, which splices a parameter into the SQL as text. cronos compiles
    parameters to bind arguments and has no path from a value to query
    structure — that is deliberate, and documented in docs/report-format.md.
    Translating `$P!{}` to a bind would change what the query means; passing it
    through would carry an injection into a system built to make it impossible.
  - A `queryString` in a language other than SQL. HQL, MDX and XPath are not
    SQL with different keywords, and a dataset that claims to hold one when it
    holds another fails at 6am rather than on import.

# Layout

An adapter, beside codec/yaml: both read a file format into domain types, and
neither is reached by the engine. The jrxml wire model is unexported — callers
get [Result], not JasperReports' idea of a band — so this package can follow
the schema without that shape leaking into anything that has to keep working.
*/
package jrxml
