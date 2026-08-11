// Package paginated renders a Document to PDF with Typst.
//
// # Why a typesetter
//
// Statements are not web pages printed. "Break the page between customers",
// "repeat the column headings after a break", "subtotal each group" and
// "number the pages of each recipient's own statement" are typesetting
// semantics. Expressed in CSS they are hints a print engine may honour; here
// they are the model.
//
// # The template is fixed
//
// The document arrives as JSON and is typeset by one embedded template. Cronos
// does not generate Typst source from a report definition, and that is a
// security decision, not a convenience one: Typst can read files and load
// images, so generated source would make every definition field a potential
// path traversal. A fixed template with data holes has no such surface.
//
// Each render also gets a private root directory containing only its own two
// files. Typst cannot address anything outside its root, so even a template
// bug cannot reach the server's filesystem.
//
// # Memory
//
// One Document is one PDF and is held whole — pagination is global, so there
// is no streaming formulation of it. The bound is the burst: 5,000 customers
// is 5,000 documents of one customer each, not one document of 5,000, so peak
// memory is the largest single recipient rather than the run.
package paginated
