package query

import (
	"fmt"
	"strings"
)

// likeEscape is the character that makes a wildcard literal.
const likeEscape = '\\'

// contains compiles a substring match.
//
// The wildcards in the caller's text are escaped first. Someone searching for
// "50%" is looking for a discount, not asking to match every row, and someone
// searching for "a_b" means the underscore. This is not a security hole — the
// value is still bound — but it is the classic way a search box returns
// everything and nobody can say why.
//
// ESCAPE is named explicitly rather than relying on a default: the backslash
// is the default in some dialects and no escape at all in others, so a search
// that behaves one way on Postgres and another on MySQL is otherwise waiting
// in the first customer who searches for a percent sign.
func (b *binder) contains(field string, v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: contains matches text", ErrBadArgument)
	}
	return fmt.Sprintf("%s LIKE %s ESCAPE '%c'",
		field, b.value("%"+escapeWildcards(s)+"%"), likeEscape), nil
}

func escapeWildcards(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for _, r := range s {
		if r == '%' || r == '_' || r == likeEscape {
			out.WriteRune(likeEscape)
		}
		out.WriteRune(r)
	}
	return out.String()
}
