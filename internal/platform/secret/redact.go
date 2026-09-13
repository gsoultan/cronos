package secret

import (
	"errors"
	"strings"
)

/*
Redact removes resolved values from an error before anything logs it.

Not naming the credential is not enough. A driver quotes back what it was
given: DuckDB prints the line it could not parse, and database/sql drivers
print the DSN they could not connect with — so an error that this package's
callers carefully built out of a name and a driver still arrives holding a
password, put there by somebody else's error message.

The caller passes what it resolved, because it is the only thing that knows.
Empty values are skipped: replacing "" would put the marker between every
character of the message.
*/
func Redact(err error, values ...string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	found := false
	for _, v := range values {
		if v == "" || !strings.Contains(msg, v) {
			continue
		}
		msg = strings.ReplaceAll(msg, v, "[redacted]")
		found = true
	}
	if !found {
		// Nothing to hide, so the error keeps its wrapping and its sentinels.
		return err
	}
	// The chain is dropped deliberately: errors.Is on it would reach an
	// Error() this function exists to keep out of reach.
	return errors.New(msg)
}
