package email

import (
	"fmt"
	"io"
	"strings"
)

// lineLimit is the longest line SMTP guarantees to carry.
//
// RFC 5321 says 1000 including CRLF; 76 is the conventional encoding width and
// leaves room for a server that rewrites headers. Exceeding it does not fail
// loudly — a relay silently folds the line and the attachment arrives corrupt.
const lineLimit = 76

// wrap writes s in lineLimit-character lines.
func wrap(w io.Writer, s string) error {
	for len(s) > lineLimit {
		if _, err := io.WriteString(w, s[:lineLimit]+"\r\n"); err != nil {
			return err
		}
		s = s[lineLimit:]
	}
	_, err := io.WriteString(w, s+"\r\n")
	return err
}

// quoted writes text as quoted-printable.
//
// Hand-rolled rather than mime/quotedprintable because that encoder does not
// guard the one case that matters here: a line beginning with "From " is
// rewritten to ">From " by some mail transfer agents, which corrupts the body
// of any statement whose first word happens to be From.
func quoted(w io.Writer, text string) error {
	var b strings.Builder
	column := 0

	for _, r := range text {
		token := encodeRune(r)
		if r == '\n' {
			b.WriteString("\r\n")
			column = 0
			continue
		}
		if column+len(token) > lineLimit-1 {
			// Soft line break: the "=" tells the decoder the line continues.
			b.WriteString("=\r\n")
			column = 0
		}
		b.WriteString(token)
		column += len(token)
	}
	_, err := io.WriteString(w, b.String()+"\r\n")
	return err
}

func encodeRune(r rune) string {
	switch {
	case r == '\r':
		return ""
	case r == '=':
		return "=3D"
	case r >= ' ' && r <= '~':
		return string(r)
	case r == '\t':
		return "=09"
	}
	// Anything else, including every non-ASCII rune, byte by byte.
	var out strings.Builder
	for _, b := range []byte(string(r)) {
		fmt.Fprintf(&out, "=%02X", b)
	}
	return out.String()
}
