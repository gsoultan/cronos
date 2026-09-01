package jrxml

import "github.com/gsoultan/cronos/internal/core/definition"

// Result is one JasperReports file, translated.
//
// Two definitions out of one file, because that is the difference between the
// formats: `.jrxml` embeds its query, and cronos separates the governed query
// from the thing that draws it, so one report becomes a Dataset and a Report.
// docs/report-format.md has the argument for why.
//
// Both are optional and the findings are not. A file whose query imported but
// whose layout could not be inferred yields a Dataset, no Report, and a Blocked
// finding saying so — which is more use than either an error or a Report with an
// invented layout.
type Result struct {
	// Source is the jasperReport name attribute, as the file spelled it. Kept
	// unnormalised: it is what somebody will grep the estate for.
	Source string

	Dataset definition.Dataset
	Report  definition.Report

	// Findings is everything not carried, worst first. Never nil for a file that
	// parsed — a Jasper report with nothing to report does not exist.
	Findings []Finding
}

// HasDataset reports whether a governed query came across.
func (r Result) HasDataset() bool { return r.Dataset.Name != "" }

// HasReport reports whether a layout came across.
func (r Result) HasReport() bool { return r.Report.Name != "" }

// Blocked reports whether this file needs a person before it is migrated.
//
// Asked of the findings rather than tracked separately, so the number an
// operator sees and the list they read cannot disagree.
func (r Result) Blocked() bool {
	for _, f := range r.Findings {
		if f.Severity == Blocked {
			return true
		}
	}
	return false
}

// Needs counts the findings at or above sev, which is what a summary line
// reports.
func (r Result) Needs(sev Severity) int {
	n := 0
	for _, f := range r.Findings {
		if !sev.worseThan(f.Severity) {
			n++
		}
	}
	return n
}
