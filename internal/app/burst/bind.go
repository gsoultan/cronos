package burst

import (
	"fmt"
	"regexp"
	"strings"
)

// hole matches {{ .row.field }} and {{ .run.field }} in a schedule's bindings.
//
// The same two sources and nothing else. A schedule is written by whoever can
// already write a dataset query, so this is not an injection boundary — it is
// a typo boundary, and a hole that resolved to empty would silently address
// five thousand emails to nobody.
var hole = regexp.MustCompile(`\{\{\s*\.(row|run)\.([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// Row is one recipient, as the `over` dataset returned them.
type Row map[string]any

// Run is what the scheduler knows about this execution: the period, the
// timestamp, the run id. Available to bindings as {{ .run.x }}.
type Run map[string]string

// resolve fills a template from the row and the run.
//
// A missing key is an error rather than an empty string. "{{ .row.emial }}"
// resolving to "" produces a delivery addressed to nowhere, and a burst that
// reports success having sent nothing is the worst outcome available.
func resolve(tmpl string, row Row, run Run) (string, error) {
	var missing []string

	out := hole.ReplaceAllStringFunc(tmpl, func(match string) string {
		parts := hole.FindStringSubmatch(match)
		source, name := parts[1], parts[2]

		if source == "run" {
			if v, ok := run[name]; ok {
				return v
			}
			missing = append(missing, ".run."+name)
			return match
		}
		if v, ok := row[name]; ok {
			return text(v)
		}
		missing = append(missing, ".row."+name)
		return match
	})

	if len(missing) > 0 {
		return "", fmt.Errorf("%w: %s", ErrBind, strings.Join(missing, ", "))
	}
	return out, nil
}

// text renders a bound value. Kept minimal: these become filenames and email
// addresses, not report figures, so the engine's formatting rules do not
// apply and should not be reached for.
func text(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	}
	return fmt.Sprint(v)
}
