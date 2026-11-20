package run

import (
	"fmt"
	"math"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// numberFormat is how one measure's figures are written: what its field in
// the dataset says it is. The zero value is plain, and writes exactly what
// compact and group always have, so a field that asks for nothing reads as
// it did before formats were applied.
type numberFormat struct {
	kind     string // "", number, currency, percent
	symbol   string // what a currency is written with: "€", or "CHF " where it has no sign of its own
	decimals int    // -1 for the format's own choice
}

// symbols are the currencies with a sign of their own. Anything else is
// written with its code, which every reader of that currency recognises.
var symbols = map[string]string{
	"USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥", "CNY": "¥", "INR": "₹",
	"KRW": "₩", "AUD": "A$", "CAD": "C$", "NZD": "NZ$", "HKD": "HK$", "SGD": "S$",
	"BRL": "R$", "MXN": "MX$", "IDR": "Rp", "ILS": "₪", "VND": "₫", "TRY": "₺",
	"PHP": "₱", "THB": "฿", "NGN": "₦", "UAH": "₴", "PLN": "zł ",
}

// wholeCurrencies are written without minor units, because they have none
// anybody spends.
var wholeCurrencies = map[string]bool{"JPY": true, "KRW": true, "IDR": true, "VND": true}

// formatOf is the format of the named field of ds, plain where it has none or
// the field is not one of the dataset's — a count of rows, say.
func formatOf(ds definition.Dataset, field string) numberFormat {
	f, ok := ds.Field(field)
	if !ok {
		return plainFormat
	}
	out := numberFormat{kind: f.Format, decimals: -1}
	if f.Decimals != nil {
		out.decimals = *f.Decimals
	}
	if out.kind == "currency" {
		out.symbol = symbolOf(f.Currency)
		if out.decimals < 0 && wholeCurrencies[f.Currency] {
			out.decimals = 0
		}
	}
	if out.kind == "preformatted" {
		out.kind = ""
	}
	return out
}

func symbolOf(code string) string {
	if s, ok := symbols[code]; ok {
		return s
	}
	if code == "" {
		return ""
	}
	return code + " "
}

// plain reports whether n writes figures the way they were written before.
func (n numberFormat) plain() bool {
	return (n.kind == "" || n.kind == "number") && n.decimals < 0
}

// short writes a figure where it is read at a glance: a tile, a bar's end, a
// tick. Millions and billions are compacted, as compact does, and money keeps
// its cents only where they are a visible part of the amount.
func (n numberFormat) short(v float64) string {
	switch {
	case n.plain():
		return compact(v)
	case n.kind == "percent":
		return n.percent(v)
	}
	sign, abs := "", v
	if v < 0 {
		sign, abs = "-", -v
	}
	var body string
	switch {
	case abs >= 1e9:
		body = trim(abs/1e9) + "B"
	case abs >= 1e6:
		body = trim(abs/1e6) + "M"
	default:
		body = group(abs, n.places(abs, true))
	}
	return sign + n.symbol + body
}

// exact writes a figure where it is read in full: a table cell. Nothing is
// compacted, and money keeps its minor units.
func (n numberFormat) exact(v float64) string {
	switch {
	case n.plain():
		return group(v, decimalsFor(v))
	case n.kind == "percent":
		return n.percent(v)
	}
	sign, abs := "", v
	if v < 0 {
		sign, abs = "-", -v
	}
	return sign + n.symbol + group(abs, n.places(abs, false))
}

// places is how many decimals a figure keeps. Money drops its cents from a
// glanced-at figure once it is in the thousands — €982,158 rather than
// €982,158.38 — and keeps them in a table, where they are the point.
func (n numberFormat) places(abs float64, glance bool) int {
	switch {
	case n.decimals >= 0:
		return n.decimals
	case n.kind == "currency" && glance && abs >= 1000:
		return 0
	case n.kind == "currency":
		return 2
	}
	return decimalsFor(abs)
}

// percent writes a fraction as a percentage, to one place unless the field
// says otherwise, and without a trailing ".0": 85%, 87.3%.
func (n numberFormat) percent(v float64) string {
	d := n.decimals
	if d < 0 {
		s := group(v*100, 1)
		return strings.TrimSuffix(s, ".0") + "%"
	}
	return group(v*100, d) + "%"
}

// change writes how far a figure moved from one period to the next. A share
// moves in points: 82% to 85% is "+3 pts", where "+3.7%" would be a change
// of a change that nobody means.
func (n numberFormat) change(last, before float64) (string, bool) {
	if n.kind == "percent" {
		pts := (last - before) * 100
		return fmt.Sprintf("%+.1f pts", pts), true
	}
	if before == 0 {
		return "", false
	}
	return fmt.Sprintf("%+.1f%%", (last-before)/math.Abs(before)*100), true
}

// plainFormat writes figures as they were always written. A spreadsheet's
// cells use it for their text, because the number beside it is the value a
// workbook formats for itself.
var plainFormat = numberFormat{decimals: -1}
