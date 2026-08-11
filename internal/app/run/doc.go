// Package run turns a report into the numbers a viewer shows.
//
// It is the use case, so it declares the ports it needs and implements none of
// them: where definitions come from and what executes a plan are decisions for
// whoever wires the binary, and this package should read the same whether the
// rows arrive from Postgres, DuckDB or a file.
//
// # Values leave here as strings
//
// Formatting happens once, on the way out, not in each of the three renderers.
// A number formatted in the browser can disagree with the same number in the
// PDF of the same report, and the one that knew the field's format is this
// one. internal/adapter/render/paginated makes the same trade.
package run
