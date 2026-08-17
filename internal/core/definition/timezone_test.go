package definition_test

import (
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
A timezone had to be non-empty and did not have to exist.

That is a way for anybody who can edit to take the deployment down. "Europe/
Berln" publishes with a 200 and the running instance carries on perfectly well,
because a schedule is parsed when the process starts and not when it is stored.
Then the next restart — a deploy, an eviction, an OOM — finds a schedule that
will not arm, and the process refuses to start. With the API down, the only way
to remove the typo is a prompt on the database, and nothing connects the outage
to a definition somebody published three weeks earlier.

Checked here, in the core, so it is the same answer wherever a definition
arrives from: the API, a directory, or a restore.
*/
func TestASchedulesTimezoneHasToExist(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		what string
		zone string
		ok   bool
	}{
		{"the demo's own", "Europe/Berlin", true},
		{"the one every deployment has", "UTC", true},
		{"another the portal offers", "Asia/Jakarta", true},
		{"one letter missing", "Europe/Berln", false},
		{"a city that is not one", "Europe/Atlantis", false},
		{"an offset, which is not a zone", "+07:00", false},
		{"a sentence", "whenever you like", false},
		// Go resolves the empty string to UTC, which is why emptiness is
		// checked separately and first: a schedule with no timezone would
		// otherwise validate and quietly mean UTC.
		{"nothing at all", "", false},
	} {
		t.Run(c.what, func(t *testing.T) {
			t.Parallel()

			err := schedule(c.zone).Validate()
			switch {
			case c.ok && err != nil:
				t.Fatalf("%q was refused: %v", c.zone, err)
			case !c.ok && err == nil:
				t.Fatalf("%q was accepted, and a restart would not come back", c.zone)
			}
			if err != nil && !strings.Contains(err.Error(), "timezone") {
				t.Fatalf("refused %q without saying it was the timezone: %v", c.zone, err)
			}
		})
	}
}

// The name is in the message, because an operator reading it has a store full
// of definitions and needs to know which one to open.
func TestTheRefusalNamesTheSchedule(t *testing.T) {
	t.Parallel()

	err := schedule("Europe/Berln").Validate()
	if err == nil {
		t.Fatal("accepted")
	}
	for _, want := range []string{"monthly-statements", "Europe/Berln"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal does not mention %q: %v", want, err)
		}
	}
}

func schedule(zone string) definition.Schedule {
	return definition.Schedule{
		Name: "monthly-statements", Report: "customer-statement", Output: "pdf",
		Cron: "0 6 1 * *", Timezone: zone,
		Deliver: []definition.DeliverSpec{{Via: "email", To: "ops@acme.example"}},
	}
}
