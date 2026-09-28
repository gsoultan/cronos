package definition

import (
	"encoding/json"
	"strings"
	"testing"
)

// A stack is written as the flag it replaced, so a definition stored before
// shares existed encodes — and so hashes — as it always has; a stack of shares
// is written by its name.
func TestStackingIsWrittenAsItWas(t *testing.T) {
	for _, c := range []struct {
		in   Block
		want string
	}{
		{Block{Chart: ColumnChart, Stacked: StackedParts}, `"stacked":true`},
		{Block{Chart: ColumnChart, Stacked: StackedShares}, `"stacked":"percent"`},
	} {
		raw, err := json.Marshal(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), c.want) {
			t.Errorf("%s written as %s, want %s in it", c.in.Stacked, raw, c.want)
		}
		var back Block
		if err := json.Unmarshal(raw, &back); err != nil || back.Stacked != c.in.Stacked {
			t.Errorf("%s read back as %q (%v)", raw, back.Stacked, err)
		}
	}
	raw, _ := json.Marshal(Block{Chart: ColumnChart})
	if strings.Contains(string(raw), "stacked") {
		t.Errorf("an unstacked chart says so: %s", raw)
	}
}

func TestStackingRefusesAWordItDoesNotKnow(t *testing.T) {
	var b Block
	if err := json.Unmarshal([]byte(`{"stacked":"sideways"}`), &b); err == nil {
		t.Error("stacked: sideways was accepted")
	}
	if err := json.Unmarshal([]byte(`{"stacked":false}`), &b); err != nil || b.Stacked.On() {
		t.Errorf("stacked: false read as %q (%v)", b.Stacked, err)
	}
}
