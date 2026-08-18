package jrxml

import "fmt"

// Finding is one thing the import did not carry, or carried differently.
//
// The honest half of an importer that covers most of a format. Every finding
// names the JasperReports construct that caused it and what happened to it, so
// the output is a work list rather than a warning nobody reads: an author with
// four hundred files needs to know which nine need opening and why.
type Finding struct {
	Severity Severity
	// Element is the JasperReports name of what caused this — `subreport`,
	// `crosstab`, `variable`, `$P!{}`. Named in the file's own vocabulary
	// because that is what the person will search the file for.
	Element string
	// Detail says what happened, in one sentence, in the active voice. It is
	// read next to hundreds of others, so it says what was dropped rather than
	// apologising for dropping it.
	Detail string
	// Count is how many of them, when the same construct occurs repeatedly. Zero
	// and one both mean once; a per-element finding for eleven fonts is noise.
	Count int
}

// String is one line of the report.
func (f Finding) String() string {
	if f.Count > 1 {
		return fmt.Sprintf("%s: %s — %s (%d)", f.Severity, f.Element, f.Detail, f.Count)
	}
	return fmt.Sprintf("%s: %s — %s", f.Severity, f.Element, f.Detail)
}

// findings accumulates findings during a translation, merging repeats.
//
// A type rather than a slice so the merge is not something each call site has to
// remember: an importer that emitted one finding per `<font>` element would
// produce a report longer than the file it describes.
type findings struct {
	list []Finding
	// at indexes by element and detail, so two occurrences of the same loss
	// become a count and two different losses on the same element stay separate.
	at map[string]int
}

func (f *findings) add(sev Severity, element, detail string) {
	f.addN(sev, element, detail, 1)
}

func (f *findings) addN(sev Severity, element, detail string, n int) {
	if n <= 0 {
		return
	}
	if f.at == nil {
		f.at = map[string]int{}
	}
	key := element + "\x00" + detail
	if i, seen := f.at[key]; seen {
		f.list[i].Count += n
		// The worst occurrence sets the grade. The same construct can be
		// cosmetic in one place and load-bearing in another.
		if sev.worseThan(f.list[i].Severity) {
			f.list[i].Severity = sev
		}
		return
	}
	f.at[key] = len(f.list)
	f.list = append(f.list, Finding{Severity: sev, Element: element, Detail: detail, Count: n})
}

// addf is add with a formatted detail.
func (f *findings) addf(sev Severity, element, format string, args ...any) {
	f.add(sev, element, fmt.Sprintf(format, args...))
}

// sorted returns the findings worst-first, stable within a severity so the
// output of two runs over the same file is identical.
func (f *findings) sorted() []Finding {
	out := append([]Finding(nil), f.list...)
	// Insertion sort on a list this short beats importing sort for a stable
	// pass, and keeps the original order within a severity.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Severity.worseThan(out[j-1].Severity); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
