package definition

import "testing"

func TestAControlThatCannotOperateItsTypeIsRefused(t *testing.T) {
	// A calendar on an enum and a dropdown on a date are both nonsense, and a
	// renderer handed one has to choose between drawing something wrong and
	// ignoring what the author asked for. Neither is an answer.
	for _, c := range []struct {
		name    string
		kind    ParamType
		control Control
		ok      bool
	}{
		{"a date takes a calendar", Date, Calendar, true},
		{"a date takes presets", Date, Presets, true},
		{"a date does not take a dropdown", Date, Dropdown, false},
		{"an enum takes a dropdown", Enum, Dropdown, true},
		{"an enum takes checkboxes", Enum, Checkboxes, true},
		{"an enum does not take a calendar", Enum, Calendar, false},
		{"a number takes a slider", Number, Slider, true},
		{"a number does not take checkboxes", Number, Checkboxes, false},
		{"a bool does not take a slider", Bool, Slider, false},
		{"nothing named is always fine", String, "", true},
		{"a control nobody implements is refused", String, "carousel", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := validateControl("f", c.kind, c.control)
			if c.ok && err != nil {
				t.Errorf("refused a valid pairing: %v", err)
			}
			if !c.ok && err == nil {
				t.Errorf("accepted %s on a %s", c.control, c.kind)
			}
		})
	}
}

func TestEveryTypeResolvesToAControlSomethingCanDraw(t *testing.T) {
	// The default is resolved once, on the server, so the portal and the embed
	// cannot disagree about what an author who named nothing gets.
	for _, kind := range []ParamType{String, Number, Bool, Date, Enum} {
		got := ControlFor(kind, "")
		if !got.Valid() {
			t.Errorf("%s defaults to %q, which nothing draws", kind, got)
		}
		if err := validateControl("f", kind, got); err != nil {
			t.Errorf("%s defaults to a control it cannot be operated by: %v", kind, err)
		}
	}
}

func TestAnAuthorsChoiceSurvivesTheDefault(t *testing.T) {
	if got := ControlFor(Date, Presets); got != Presets {
		t.Errorf("ControlFor overrode the author with %q", got)
	}
}
