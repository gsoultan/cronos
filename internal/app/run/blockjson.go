package run

import "encoding/json"

// MarshalJSON emits only the fields the block's kind owns, and always emits
// those.
//
// Neither of the obvious spellings works. Plain tags put `"series": null` on
// every stat tile — noise a client has to know to ignore. `omitempty` drops an
// *empty* collection too, so a chart that matched no rows loses the key
// entirely and a client reading `series.map` crashes on the emptiest, most
// ordinary case there is.
//
// A client should never have to distinguish absent from empty, so for the kind
// that owns a collection it is always there, and for the kinds that do not it
// never is.
func (b Block) MarshalJSON() ([]byte, error) {
	out := map[string]any{"kind": b.Kind, "title": b.Title}
	if b.Coverage != nil {
		out["coverage"] = b.Coverage
	}

	switch b.Kind {
	case "stat", "text":
		out["value"] = b.Value
	case "chart":
		out["chart"] = b.Chart
		out["series"] = nonNilBars(b.Series)
	case "table":
		out["columns"] = nonNilColumns(b.Columns)
		out["rows"] = nonNilRows(b.Rows)
		out["total"] = b.Total
	}
	return json.Marshal(out)
}

func nonNilBars(v []Bar) []Bar {
	if v == nil {
		return []Bar{}
	}
	return v
}

func nonNilColumns(v []Column) []Column {
	if v == nil {
		return []Column{}
	}
	return v
}

func nonNilRows(v [][]string) [][]string {
	if v == nil {
		return [][]string{}
	}
	return v
}
