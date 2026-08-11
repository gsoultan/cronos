package query

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// resolve turns a caller's arguments into one value per declared parameter.
//
// Unknown keys are an error rather than being ignored. A caller who sends
// `custmer_id` and is answered with the whole table has been told their filter
// worked; the only kind thing to do with a typo is to say so.
func resolve(ds definition.Dataset, in map[string]any, now func() time.Time) (map[string]any, error) {
	for name := range in {
		if _, ok := ds.Param(name); !ok {
			return nil, fmt.Errorf("%w: %q is not a parameter of this dataset", ErrBadArgument, name)
		}
	}

	out := make(map[string]any, len(ds.Params))
	for _, p := range ds.Params {
		raw, sent := in[p.Name]
		if !sent {
			if !p.HasDefault() && p.Required {
				return nil, fmt.Errorf("%w: %q is required", ErrBadArgument, p.Name)
			}
			raw = p.Default
		}
		if raw == nil {
			out[p.Name] = nil
			continue
		}
		v, err := coerce(p, raw, now)
		if err != nil {
			return nil, err
		}
		out[p.Name] = v
	}
	return out, nil
}

func coerce(p definition.Param, raw any, now func() time.Time) (any, error) {
	if !p.Multiple {
		return coerceOne(p, raw, now)
	}
	list, ok := raw.([]any)
	if !ok {
		if strs, isStrs := raw.([]string); isStrs {
			list = make([]any, len(strs))
			for i, s := range strs {
				list[i] = s
			}
		} else {
			return nil, fmt.Errorf("%w: %q takes a list", ErrBadArgument, p.Name)
		}
	}
	out := make([]any, len(list))
	for i, item := range list {
		v, err := coerceOne(p, item, now)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func coerceOne(p definition.Param, raw any, now func() time.Time) (any, error) {
	switch p.Type {
	case definition.String:
		return asString(p, raw)
	case definition.Bool:
		b, ok := raw.(bool)
		if !ok {
			return nil, typeErr(p, "true or false")
		}
		return b, nil
	case definition.Number:
		return asNumber(p, raw)
	case definition.Date:
		return asDate(p, raw, now)
	case definition.Enum:
		return asEnum(p, raw)
	}
	return nil, fmt.Errorf("%w: %q has unknown type %q", ErrBadArgument, p.Name, p.Type)
}

func asString(p definition.Param, raw any) (any, error) {
	s, ok := raw.(string)
	if !ok {
		return nil, typeErr(p, "text")
	}
	return s, nil
}

func asNumber(p definition.Param, raw any) (any, error) {
	switch n := raw.(type) {
	case float64:
		return n, nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return nil, typeErr(p, "a number")
		}
		return f, nil
	}
	return nil, typeErr(p, "a number")
}

func asEnum(p definition.Param, raw any) (any, error) {
	s, ok := raw.(string)
	if !ok {
		return nil, typeErr(p, "one of the listed values")
	}
	for _, allowed := range p.Values {
		if s == allowed {
			return s, nil
		}
	}
	// The permitted values are named back. They are the author's declaration,
	// not data, so echoing them discloses nothing a caller could not read in
	// the dataset's own schema.
	return nil, fmt.Errorf("%w: %q is not a value of %q, want one of %v",
		ErrBadArgument, s, p.Name, p.Values)
}

func typeErr(p definition.Param, want string) error {
	// The value itself is never quoted back. An error page that echoes input
	// is a probe oracle, and worse, a reflected-content vector in whatever
	// renders it.
	return fmt.Errorf("%w: %q wants %s", ErrBadArgument, p.Name, want)
}
