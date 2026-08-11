package history

import (
	"strings"
	"testing"
	"time"
)

func TestIDsSortByTimeAndDoNotCollide(t *testing.T) {
	base := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)

	earlier := NewID(base)
	later := NewID(base.Add(time.Hour))
	if !(earlier < later) {
		t.Errorf("%s should sort before %s", earlier, later)
	}

	// Two schedules firing in the same second is the normal case at six on the
	// first of the month.
	seen := map[string]bool{}
	for range 1000 {
		id := NewID(base)
		if seen[id] {
			t.Fatalf("collision after %d: %s", len(seen), id)
		}
		seen[id] = true
	}
}

// It appears in URLs and support tickets.
func TestAnIDIsReadableAloud(t *testing.T) {
	id := NewID(time.Now())
	if !strings.HasPrefix(id, "run_") || len(id) > 24 {
		t.Errorf("id = %q", id)
	}
}
