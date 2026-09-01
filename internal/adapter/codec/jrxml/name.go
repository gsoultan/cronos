package jrxml

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

/*
Names have to change on the way in, and the three kinds change differently.

cronos constrains names: a definition is `^[a-z][a-z0-9-]*$` and a param or
field is `^[a-z][a-z0-9_]*$`, both enforced by definition.Validate. Jasper
constrains nothing — `Invoice_Statement`, `FROM_DATE` and `Total Amount` are all
legal there. So something has to give, and what may safely give depends on
where the name is read:

  - A **definition name** is an address. Nothing outside this import refers to
    it yet, so it can be rewritten freely: `Invoice_Statement` becomes
    `invoice-statement`.

  - A **param name** is bound, never interpolated. `$P{FROM_DATE}` compiles to a
    placeholder and the name reaches SQL only in error messages, so it too can be
    rewritten freely: `FROM_DATE` becomes `from_date`.

  - A **field name** reaches SQL as text. query.tableSQL writes
    `SELECT customer_name FROM (…)` around the dataset's query, so a field's name
    must still name a column that query returns. This is the one that cannot be
    prettified: `CustomerName` may only become `customername`, because
    `customer_name` is a column that does not exist.

The residual risk in that last case is a quoted alias. `SELECT c.name AS
CustomerName` yields a column Postgres folds to `customername` and MySQL, SQLite
and SQL Server match case-insensitively, so lower-casing is safe. `AS
"CustomerName"` yields a column only that exact spelling finds, and cronos cannot
spell it. That is a finding rather than a silent rename — see fieldName.
*/

// fallbackName is what a definition is called when nothing in its Jasper name
// survives the reduction to a cronos identifier.
//
// Reachable: a report named entirely in Cyrillic or Chinese has no letters the
// name rule admits. It imports rather than failing, and dataset() reports it,
// because a catalog entry called `report-7` is a rename somebody has to make
// rather than a migration they have to redo.
const fallbackName = "report"

// slugify turns a Jasper report name into a definition name.
func slugify(s string) string {
	return join(words(s), '-', fallbackName)
}

// paramName turns a Jasper parameter name into a cronos identifier.
//
// Free to be readable, because a param name never reaches SQL as text.
func paramName(s string) string {
	return join(words(s), '_', "param")
}

// fieldName lower-cases a Jasper field name, and says whether that was enough.
//
// ok false means the name needed more than case folding to become an
// identifier, so the emitted field no longer names the column the query
// returns. The caller reports it: the fix is an alias in the SQL, and only the
// author knows whether the column was quoted.
func fieldName(s string) (name string, ok bool) {
	lower := strings.ToLower(strings.TrimSpace(s))
	if identifierLike(lower) {
		return lower, true
	}
	return join(words(s), '_', "field"), false
}

// identifierLike reports whether s already satisfies definition's identifier
// rule, so lower-casing was the whole change.
func identifierLike(s string) bool {
	if s == "" || !(s[0] >= 'a' && s[0] <= 'z') {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

// words splits a name at every boundary a human would read as one.
//
// Case transitions count: `CustomerName` is two words and `customer_name` is the
// same two, so both arrive at the same answer. `HTTPStatus` is two as well —
// the run of capitals ends one word before the last capital starts the next.
func words(s string) []string {
	var (
		out   []string
		cur   strings.Builder
		runes = []rune(s)
	)
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, strings.ToLower(cur.String()))
			cur.Reset()
		}
	}
	for i, r := range runes {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			// A capital starting a new word: either the previous rune was
			// lower-case or a digit, or this is the last capital of a run
			// followed by a lower-case letter.
			if unicode.IsUpper(r) && i > 0 {
				prev := runes[i-1]
				next := rune(0)
				if i+1 < len(runes) {
					next = runes[i+1]
				}
				if unicode.IsLower(prev) || unicode.IsDigit(prev) ||
					(unicode.IsUpper(prev) && unicode.IsLower(next)) {
					flush()
				}
			}
			cur.WriteRune(r)
		default:
			// Spaces, underscores, dots, dashes, and anything non-ASCII that is
			// neither letter nor digit.
			flush()
		}
	}
	flush()
	return out
}

// join assembles words with sep, guaranteeing a leading lower-case letter.
func join(parts []string, sep byte, fallback string) string {
	var b strings.Builder
	for _, p := range parts {
		clean := asciify(p)
		if clean == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(sep)
		}
		b.WriteString(clean)
	}
	out := b.String()
	if out == "" {
		return fallback
	}
	if out[0] >= '0' && out[0] <= '9' {
		// A name has to start with a letter. Prefixing beats dropping the
		// digit, which would turn `2024_sales` into `024-sales`.
		return fallback + string(sep) + out
	}
	return out
}

// asciify reduces a word to the ASCII letters and digits an identifier may hold.
//
// By canonical decomposition rather than by dropping: `Müller` has to become
// `muller`, and dropping the letter it cannot spell gives `mller`, which is not
// a name anybody recognises as theirs. NFD separates `ü` into `u` and a
// combining diaeresis, and discarding the mark is a defined transformation
// rather than a guess about a language.
//
// The replacer ahead of it covers the Latin letters that have no decomposition —
// `ß`, `ø`, `æ` — where the conventional ASCII spelling is one-to-many and NFD
// leaves them whole. A script with no Latin fallback at all, Cyrillic or CJK,
// still comes out empty; join falls back to a generic name and dataset() reports
// it, because a report called `report-7` needs someone to rename it.
func asciify(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(latin.Replace(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// A combining mark left over from the decomposition.
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		}
	}
	return b.String()
}

// latin spells the letters NFD cannot decompose, the way the languages that use
// them spell them in ASCII.
var latin = strings.NewReplacer(
	"ß", "ss", "ẞ", "ss", "ø", "o", "Ø", "o", "æ", "ae", "Æ", "ae",
	"œ", "oe", "Œ", "oe", "đ", "d", "Đ", "d", "ł", "l", "Ł", "l",
	"þ", "th", "Þ", "th", "ð", "d", "Ð", "d", "ı", "i",
)

// unique hands out names that do not collide, remembering what it gave out.
//
// Needed because normalisation is lossy in both directions: two Jasper reports
// called `Sales Summary` and `sales-summary` land on one definition name, and so
// do `FROM_DATE` and `fromDate`. Silently merging them would make one parameter
// out of two and bind the wrong value.
type unique struct {
	taken map[string]bool
}

// pick returns want, or want with a numeric suffix, and whether it had to.
func (u *unique) pick(want string, sep byte) (string, bool) {
	if u.taken == nil {
		u.taken = map[string]bool{}
	}
	if !u.taken[want] {
		u.taken[want] = true
		return want, false
	}
	for n := 2; ; n++ {
		candidate := want + string(sep) + itoa(n)
		if !u.taken[candidate] {
			u.taken[candidate] = true
			return candidate, true
		}
	}
}

// itoa avoids importing strconv for one call in a package that formats nothing
// else numerically.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
