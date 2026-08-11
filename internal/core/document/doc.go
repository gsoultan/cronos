// Package document is a paginated document: what a statement *is*, before
// anything decides how to typeset it.
//
// Core rather than the renderer, because a burst produces one of these per
// recipient and the use case that does so must not depend on which typesetter
// draws it. internal/adapter/render/paginated is one implementation; a
// spreadsheet export reading the same groups would be another.
//
// # Every cell is already a string
//
// Formatting money, dates and locales is the engine's job. Doing it here
// rather than in each renderer keeps rounding in one place — a template that
// formats currency is a second implementation of the rules that decided what
// to bill, and the two will disagree eventually.
package document
