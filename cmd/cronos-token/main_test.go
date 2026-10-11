package main

import (
	"testing"

	"github.com/gsoultan/cronos/internal/platform/config"
)

// A token minted with no -org or -project has to name the project cronosd
// serves under the same environment, or the README's quickstart is refused.
func TestMintsForTheProjectTheServerServes(t *testing.T) {
	for _, env := range [][2]string{{"", ""}, {"acme", "finance"}} {
		t.Setenv("CRONOS_ORG", env[0])
		t.Setenv("CRONOS_PROJECT", env[1])
		t.Setenv("CRONOS_SIGNING_KEY", "development-key-at-least-32-bytes-long")
		s, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		if got := tenant("CRONOS_ORG"); got != s.Org {
			t.Errorf("CRONOS_ORG=%q: token org %q, server org %q", env[0], got, s.Org)
		}
		if got := tenant("CRONOS_PROJECT"); got != s.Project {
			t.Errorf("CRONOS_PROJECT=%q: token project %q, server project %q", env[1], got, s.Project)
		}
	}
}
