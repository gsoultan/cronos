// Package api serves the embed endpoint.
//
// The one rule here: what a caller is told is decided in this package, and it
// is never what the database said. A driver error names tables, columns and
// sometimes values — useful in a log, a schema disclosure in a response body.
// Errors are mapped to a status and a sentence, and the detail goes to the
// operator.
package api
