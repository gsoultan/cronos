// Package spreadsheet writes XLSX.
//
// # By hand
//
// An .xlsx is a zip of four small XML files, and what a report needs from it —
// a sheet of cells, a frozen header, an autofilter — is a few hundred lines of
// well-specified markup. A library for that is fifteen megabytes of module and
// a styling API to learn, in exchange for features a report engine does not
// use: charts, pivot tables, formulas, images.
//
// The bet is only reasonable because it is checked. spreadsheet_test.go opens
// every file this package produces with openpyxl, a real reader, and asserts
// on the values it finds. "It probably parses" would not be worth the saving.
//
// # Numbers stay numbers
//
// A total written as text is a total nobody can sum in the tool they opened it
// in, which is the entire reason they asked for a spreadsheet rather than a
// PDF. Cells carry a type, and the numeric ones are written unformatted so the
// recipient's own locale decides how they look.
package spreadsheet
