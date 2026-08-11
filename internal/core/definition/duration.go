package definition

import (
	"fmt"
	"time"
)

// Duration is a time.Duration that reads "30s" in YAML.
//
// time.Duration is an int64, so a plain field would make a definition say
// `statementTimeout: 30000000000`. Authors write durations the way they say
// them.
type Duration time.Duration

// UnmarshalYAML parses "30s", "5m", "2h".
func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var text string
	if err := unmarshal(&text); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("%q is not a duration — try 30s, 5m, 2h", text)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML writes it back the way it was written.
func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

// String makes it readable in a log line.
func (d Duration) String() string { return time.Duration(d).String() }
