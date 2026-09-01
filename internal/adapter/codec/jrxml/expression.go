package jrxml

import (
	"fmt"
	"strings"
)

// ref is one $P{}, $F{} or $V{} occurrence in a Jasper expression.
type ref struct {
	// sigil is 'P' for a parameter, 'F' for a field, 'V' for a variable.
	sigil byte
	name  string
	// splice marks `$P!{x}`, which pastes the parameter into the SQL as text
	// rather than binding it. Refused — see refuseSplice.
	splice bool
	// start and end bound the whole token in the source string.
	start, end int
}

// scanRefs finds every $X{...} token in s, in order.
//
// A scanner rather than a regular expression because the interesting case is
// `$P!{`: a pattern loose enough to match both forms tends to match the bang as
// optional and then nobody notices which one they got, and the two mean
// materially different things.
func scanRefs(s string) []ref {
	var out []ref
	for i := 0; i+1 < len(s); i++ {
		if s[i] != '$' {
			continue
		}
		sigil := s[i+1]
		if sigil != 'P' && sigil != 'F' && sigil != 'V' {
			continue
		}
		j := i + 2
		splice := false
		if j < len(s) && s[j] == '!' {
			splice = true
			j++
		}
		if j >= len(s) || s[j] != '{' {
			continue
		}
		close := strings.IndexByte(s[j:], '}')
		if close < 0 {
			continue
		}
		end := j + close + 1
		out = append(out, ref{
			sigil: sigil, name: strings.TrimSpace(s[j+1 : end-1]),
			splice: splice, start: i, end: end,
		})
		i = end - 1
	}
	return out
}

// translateQuery rewrites a Jasper query into a cronos one.
//
// `$P{x}` becomes `{{ .params.x }}`, which the builder compiles to a bind
// placeholder. names maps a Jasper parameter name to the identifier the emitted
// dataset declares, so the two sides agree after normalisation.
func translateQuery(sql string, names map[string]string) (string, error) {
	refs := scanRefs(sql)
	var b strings.Builder
	last := 0
	for _, r := range refs {
		if r.splice {
			return "", refuseSplice(r)
		}
		switch r.sigil {
		case 'F', 'V':
			// Jasper does not allow these in a query either; a file that has
			// one has a query that never ran.
			return "", fmt.Errorf("%w: the query reads $%c{%s}, which is not a parameter",
				ErrRefused, r.sigil, r.name)
		}
		name, known := names[r.name]
		if !known {
			return "", fmt.Errorf("%w: the query reads $P{%s}, which the report does not declare",
				ErrRefused, r.name)
		}
		b.WriteString(sql[last:r.start])
		b.WriteString("{{ .params.")
		b.WriteString(name)
		b.WriteString(" }}")
		last = r.end
	}
	b.WriteString(sql[last:])
	return b.String(), nil
}

// refuseSplice explains the one refusal a migrating team is most likely to hit,
// and the only one that is a security decision rather than a capability gap.
//
// `$P!{sortColumn}` is how a Jasper report makes its ORDER BY configurable, and
// it works by concatenating the caller's string into the SQL. cronos has no such
// path, deliberately: docs/report-format.md commits to parameters being bound
// and to enum-with-fixed-fragments as the only way a parameter changes query
// structure. Converting it to a bind would silently produce a query that orders
// by a constant; passing it through would carry the injection into a system
// built to make it impossible. So it stops here, with the fix named.
func refuseSplice(r ref) error {
	return fmt.Errorf("%w: the query splices $P!{%s} into the SQL as text. "+
		"cronos binds parameters and has no path from a value to query structure, so this "+
		"cannot be imported as written — declare the alternatives as an enum parameter and "+
		"write one fixed SQL fragment per value, or split the report per variant",
		ErrRefused, r.name)
}

// plainRef reads an expression that is exactly one reference and nothing else.
//
// The whole basis of the layout inference. `$F{total}` is a column;
// `$F{qty} * $F{price}` is arithmetic Jasper did in Java, and cronos has no
// block that computes — the equivalent is an expression in the dataset's SELECT,
// which only the author can write. So this is deliberately strict: anything with
// a second token, an operator or a method call is not a column reference, and
// pretending otherwise imports a column of wrong numbers.
func plainRef(expr string) (r ref, ok bool) {
	trimmed := unwrapToString(strings.TrimSpace(expr))
	refs := scanRefs(trimmed)
	if len(refs) != 1 {
		return ref{}, false
	}
	only := refs[0]
	if only.splice || only.name == "" {
		return ref{}, false
	}
	// Nothing before it and nothing after it.
	if only.start != 0 || only.end != len(trimmed) {
		return ref{}, false
	}
	return only, true
}

// unwrapToString drops a trailing `.toString()` from a reference.
//
// `$F{OrderId}.toString()` is the same column as `$F{OrderId}`. The call
// changes how Java rendered it and not which field it is, and this importer
// does not carry rendering anyway — a cronos column formats from the field's
// declared type. Found in JasperReports' own samples, where dropping it meant
// dropping the column: a report short one column is a worse import than one
// whose number is formatted by a different rule.
func unwrapToString(s string) string {
	for {
		t := strings.TrimSuffix(s, ".toString()")
		if t == s {
			return s
		}
		s = strings.TrimSpace(t)
	}
}

// literalString reads a Java string literal, which is how Jasper spells a fixed
// chart series name or a static title.
//
// Returns ok false for anything else — including a concatenation, which is the
// common case and is not a literal however much of it is quoted.
func literalString(expr string) (string, bool) {
	s := strings.TrimSpace(expr)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return "", false
	}
	inner := s[1 : len(s)-1]
	// A quote inside means the literal ended early and something follows it.
	if strings.Contains(inner, `"`) {
		return "", false
	}
	// Java escapes. Only the ones that appear in a report title are worth
	// reading; anything else stays as written, which is visible rather than
	// wrong.
	if strings.Contains(inner, `\`) {
		inner = strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\n`, "\n", `\t`, "\t").Replace(inner)
	}
	return inner, true
}

// javaDefault reads a Jasper defaultValueExpression as a cronos param default.
//
// Only the shapes that are values. `new Date()` is a date and today is what it
// means; `new SimpleDateFormat("yyyy").format(new Date())` is a computation, and
// a default that is quietly wrong is worse than one the author has to supply.
func javaDefault(expr string) (any, bool) {
	s := strings.TrimSpace(expr)
	if s == "" {
		return nil, false
	}
	if v, ok := literalString(s); ok {
		return v, true
	}
	switch s {
	case "new Date()", "new java.util.Date()", "today()", "TODAY()":
		// query.relative resolves "today" against the run's clock, which is what
		// a scheduled report needs — see internal/core/query/date.go.
		return "today", true
	case "Boolean.TRUE", "true":
		return true, true
	case "Boolean.FALSE", "false":
		return false, true
	}
	// A bare number, possibly with a Java type suffix or a wrapper call.
	if n, ok := javaNumber(s); ok {
		return n, true
	}
	return nil, false
}

// javaNumber reads the numeric literal shapes a default value takes.
func javaNumber(s string) (any, bool) {
	// new BigDecimal("0"), Integer.valueOf(0), new Long(5)
	for _, wrapper := range []string{
		"new BigDecimal(", "new java.math.BigDecimal(", "Integer.valueOf(",
		"Long.valueOf(", "Double.valueOf(", "new Integer(", "new Long(", "new Double(",
	} {
		if strings.HasPrefix(s, wrapper) && strings.HasSuffix(s, ")") {
			inner := strings.TrimSuffix(strings.TrimPrefix(s, wrapper), ")")
			if lit, ok := literalString(inner); ok {
				return numberOf(lit)
			}
			return numberOf(strings.TrimSpace(inner))
		}
	}
	return numberOf(s)
}

// numberOf parses a decimal literal, tolerating Java's type suffixes.
//
// Returned as a string when it has a fractional part, because YAML would write
// a float and a monetary default is not a float — the engine coerces a param
// default through query.asNumber, which reads either.
func numberOf(s string) (any, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "LlFfDd")
	if s == "" {
		return nil, false
	}
	body := strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if body == "" {
		return nil, false
	}
	dots := 0
	for i := 0; i < len(body); i++ {
		switch c := body[i]; {
		case c >= '0' && c <= '9':
		case c == '.':
			dots++
			if dots > 1 {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if dots == 1 {
		return s, true
	}
	n := 0
	neg := strings.HasPrefix(s, "-")
	for i := 0; i < len(body); i++ {
		n = n*10 + int(body[i]-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}
