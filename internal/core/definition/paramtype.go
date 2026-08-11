package definition

// ParamType is the declared type of a dataset parameter.
//
// The type is what turns a caller's JSON into something safe to bind. It is
// not decoration: an API caller sends `{"from": "2026-07-01"}` and the type is
// the only thing that says whether that is a date, a string that looks like
// one, or an attempt at something else.
type ParamType string

const (
	String ParamType = "string"
	Number ParamType = "number"
	Bool   ParamType = "bool"
	Date   ParamType = "date"
	Enum   ParamType = "enum"
)

// Valid reports whether t is a type the engine knows how to bind.
func (t ParamType) Valid() bool {
	switch t {
	case String, Number, Bool, Date, Enum:
		return true
	}
	return false
}
