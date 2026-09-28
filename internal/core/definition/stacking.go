package definition

import (
	"encoding/json"
	"fmt"
)

// Stacking is how a multi-series chart's series share a bucket.
//
// Written `stacked: true` for parts end to end, and `stacked: percent` for
// parts as shares of their bucket's whole — the 100% stack, which answers
// "what share" where a stack answers "how much". One field rather than a
// second flag beside the first, because a share that is not stacked is a
// combination nobody means.
type Stacking string

const (
	// NotStacked draws series beside each other. The zero value.
	NotStacked Stacking = ""
	// StackedParts draws a bucket's parts end to end.
	StackedParts Stacking = "stacked"
	// StackedShares draws them as shares of their bucket's whole.
	StackedShares Stacking = "percent"
)

// On reports whether the series stack at all.
func (s Stacking) On() bool { return s != NotStacked }

// Shares reports whether each part is drawn as its share of the whole.
func (s Stacking) Shares() bool { return s == StackedShares }

// UnmarshalYAML reads `true`, `false` or `percent`.
func (s *Stacking) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	return s.read(raw)
}

// MarshalYAML writes it the way it is written.
func (s Stacking) MarshalYAML() (any, error) { return s.written(), nil }

// UnmarshalJSON reads the same three.
func (s *Stacking) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	return s.read(raw)
}

// MarshalJSON writes `true` for a stack, as the flag this replaced did: a
// definition stored before `percent` existed encodes, and so hashes, as it
// always has.
func (s Stacking) MarshalJSON() ([]byte, error) { return json.Marshal(s.written()) }

func (s Stacking) written() any {
	switch s {
	case NotStacked:
		return false
	case StackedParts:
		return true
	}
	return string(s)
}

func (s *Stacking) read(raw any) error {
	switch v := raw.(type) {
	case nil:
		*s = NotStacked
	case bool:
		*s = NotStacked
		if v {
			*s = StackedParts
		}
	case string:
		if v != string(StackedShares) {
			return fmt.Errorf("stacked is %q — want true, false or percent", v)
		}
		*s = StackedShares
	default:
		return fmt.Errorf("stacked is %v — want true, false or percent", raw)
	}
	return nil
}
