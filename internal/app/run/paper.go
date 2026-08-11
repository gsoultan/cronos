package run

import (
	"strconv"
	"strings"
)

// paper maps what an author writes to what the renderer's allow-list holds.
//
// A definition says "A4" because that is how a person writes it; the renderer
// says "a4" because that is Typst's name. Lower-casing here rather than
// widening the allow-list keeps the renderer's list of papers exact.
func paper(size string) string {
	switch strings.ToLower(strings.TrimSpace(size)) {
	case "", "a4":
		return "a4"
	case "letter", "us-letter":
		return "us-letter"
	case "legal", "us-legal":
		return "us-legal"
	case "a3", "a5", "us-tabloid":
		return strings.ToLower(size)
	}
	// Passed through so the renderer refuses it by name. Silently substituting
	// A4 for a paper somebody asked for would print the wrong document.
	return strings.ToLower(size)
}

// millimetres parses "20mm" into a number.
//
// Parsed here rather than in the template, because the only tool a typesetter
// offers for it is `eval` on a definition-supplied string — arbitrary code
// execution in the renderer. A unit nobody recognises yields zero, which the
// renderer reads as its own default rather than as no margin.
func millimetres(s string) float64 {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasSuffix(s, "mm"):
		return decimal(strings.TrimSuffix(s, "mm"))
	case strings.HasSuffix(s, "cm"):
		return decimal(strings.TrimSuffix(s, "cm")) * 10
	case strings.HasSuffix(s, "in"):
		return decimal(strings.TrimSuffix(s, "in")) * 25.4
	}
	return 0
}

func decimal(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}
