package definition

import "fmt"

// Control is which interface a filter is operated through.
//
// The author chooses, not the renderer. A status with four values and a status
// with forty want different controls, and only the person who knows the data
// knows which — so the choice lives in the definition and every renderer
// honours it, rather than each one guessing from the type and guessing
// differently.
//
// Empty means the default for the filter's type, which is what most filters
// want and why this is optional.
type Control string

const (
	// Calendar picks one date.
	Calendar Control = "calendar"
	// Range takes two ends — dates or numbers. Either alone narrows.
	Range Control = "range"
	// Presets are relative periods: the last 7, 30 or 90 days, this month.
	Presets Control = "presets"
	// Dropdown is a list that opens.
	Dropdown Control = "dropdown"
	// Radio shows every option at once, one choosable.
	Radio Control = "radio"
	// Checkboxes shows every option at once, several choosable.
	Checkboxes Control = "checkboxes"
	// Search matches text anywhere in the value.
	Search Control = "search"
	// Slider drags a number between the ends the data has.
	Slider Control = "slider"
)

// controls is every control, in the order an error message should list them.
var controls = []Control{
	Calendar, Range, Presets, Dropdown, Radio, Checkboxes, Search, Slider,
}

// Valid reports whether c is a control every renderer implements.
func (c Control) Valid() bool {
	for _, k := range controls {
		if c == k {
			return true
		}
	}
	return false
}

// suits maps each filter type to the controls that can operate it.
//
// A calendar on an enum and a dropdown on a date are both nonsense, and the
// renderer that received one would have to decide between drawing something
// wrong and ignoring what the author asked for. Refusing it here means neither
// happens.
var suits = map[ParamType][]Control{
	Date:   {Range, Calendar, Presets},
	Number: {Range, Slider, Search},
	Enum:   {Dropdown, Radio, Checkboxes},
	Bool:   {Dropdown, Radio},
	String: {Search},
}

// ControlFor is the control to render: the author's, or the type's default.
//
// The default is the first of the suitable ones, so the list above is also the
// preference order — and a type gaining a control does not silently change what
// existing filters render as.
func ControlFor(t ParamType, chosen Control) Control {
	if chosen != "" {
		return chosen
	}
	if ok := suits[t]; len(ok) > 0 {
		return ok[0]
	}
	return Search
}

// validateControl reports why this control cannot operate this type.
func validateControl(name string, t ParamType, c Control) error {
	if c == "" {
		return nil
	}
	if !c.Valid() {
		return fmt.Errorf("%w: filter %q has control %q, want one of %v",
			ErrInvalid, name, c, ControlNames())
	}
	for _, ok := range suits[t] {
		if c == ok {
			return nil
		}
	}
	return fmt.Errorf("%w: filter %q is a %s and cannot be operated by a %s — %s takes %v",
		ErrInvalid, name, t, c, t, suits[t])
}

// ControlNames lists every valid control, for an error message or a form.
func ControlNames() []string {
	out := make([]string, len(controls))
	for i, c := range controls {
		out[i] = string(c)
	}
	return out
}

// ControlsFor lists the controls a type accepts, for a form.
func ControlsFor(t ParamType) []Control { return suits[t] }
