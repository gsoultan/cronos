package run

import (
	"fmt"
	"strconv"
	"strings"
)

// Formatting lives here so a number reads the same in a browser, a PDF and a
// spreadsheet. Doing it per renderer is how the tile and the document that
// followed it end up disagreeing about the same total.

// compact renders a headline number: 49_871_204.55 becomes "49.9M".
//
// A stat tile is read at a glance and the last six digits of a revenue figure
// are never the point. The table below it carries the exact value for anyone
// who needs it.
func compact(v float64) string {
	abs := v
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= 1e9:
		return trim(v/1e9) + "B"
	case abs >= 1e6:
		return trim(v/1e6) + "M"
	}
	// No thousands tier. Compacting is per value, so a threshold anywhere puts
	// two tiles side by side reading "13K" and "4,000" — and below a million
	// the grouped figure is short enough to read anyway. Millions is where the
	// exact digits stop being the point.
	return group(v, decimalsFor(v))
}

func trim(v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}

// group renders a number with thousands separators.
//
// Grouped rather than bare: 49871204 and 4987120 are the same shape at a
// glance, and telling them apart is the whole job of a figure in a report.
func group(v float64, decimals int) string {
	s := strconv.FormatFloat(v, 'f', decimals, 64)

	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	whole, frac, _ := strings.Cut(s, ".")

	var out strings.Builder
	for i, d := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(d)
	}
	if frac != "" {
		out.WriteByte('.')
		out.WriteString(frac)
	}
	return sign + out.String()
}

// decimalsFor keeps whole numbers whole. A count of invoices is not 463.00.
func decimalsFor(v float64) int {
	if v == float64(int64(v)) {
		return 0
	}
	return 2
}

// cell renders one table value.
//
// The database's own type decides. A date arrives as a string and stays one; a
// number is grouped; a null is an em dash rather than the word "null" or an
// empty cell that reads as zero.
func cell(v any) string {
	switch t := v.(type) {
	case nil:
		return "—"
	case []byte:
		return string(t)
	case string:
		return t
	case bool:
		if t {
			return "Yes"
		}
		return "No"
	case float64:
		return group(t, decimalsFor(t))
	case float32:
		return group(float64(t), decimalsFor(float64(t)))
	case int64:
		return group(float64(t), 0)
	case int:
		return group(float64(t), 0)
	}
	return fmt.Sprint(v)
}

// number coerces whatever a driver returned into a float.
//
// Drivers disagree about aggregates: an integer SUM comes back int64 from one
// and []byte from another, and a stat tile that renders "[52 57]" is the usual
// symptom. Returning ok rather than zero keeps "nothing matched" distinct from
// "the driver surprised us".
func number(v any) (float64, bool) {
	switch t := v.(type) {
	case nil:
		return 0, false
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int64:
		return float64(t), true
	case int:
		return float64(t), true
	case []byte:
		f, err := strconv.ParseFloat(string(t), 64)
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	}
	return 0, false
}
