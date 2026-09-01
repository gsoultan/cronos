package file

import (
	"strings"
	"testing"
)

/*
One stored definition this build will not accept, and the rest that it will.

Adopt is all or nothing, which is right for the ordinary path. What was wrong
was what happened when it refused: boot returned the error and the process did
not start — so a single definition the store held could take every report in
every project off the air, and with the API down the only way to remove it was a
prompt on the database.

That is not hypothetical, because validation gets stricter and the store
outlives any one build. A schedule published against a timezone nobody checked
at the time became, one release later, a deployment that would not start — on
the upgrade, which is when nobody is looking for a definition somebody published
in March.

AdoptUsable is the fallback. Every check here is about the same thing: it takes
what it can, it says exactly what it did not, and it never leaves the caller
believing everything is fine.
*/

const goodReport = `
apiVersion: cronos.dev/v1
kind: Report
metadata:
  name: billing-summary
spec:
  dataset: invoices
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: stat
          label: Total billed
          value: {field: total, aggregate: sum}
`

const badSchedule = `
apiVersion: cronos.dev/v1
kind: Schedule
metadata:
  name: typo-schedule
spec:
  report: billing-summary
  output: pdf
  cron: "0 6 1 * *"
  timezone: Europe/Berln
  deliver:
    - via: email
      to: ops@acme.example
`

func TestAdoptIsStillAllOrNothing(t *testing.T) {
	r := empty()
	if err := r.Adopt([][]byte{[]byte(goodReport), []byte(badSchedule)}); err == nil {
		t.Fatal("Adopt took a document it should have refused")
	}
	// And it changed nothing, which is what "all or nothing" means.
	if got := len(r.Reports()); got != 0 {
		t.Fatalf("a refused Adopt left %d reports behind", got)
	}
}

func TestAdoptUsableTakesTheGoodAndNamesTheBad(t *testing.T) {
	r := empty()

	refused := r.AdoptUsable([][]byte{[]byte(goodReport), []byte(badSchedule)})

	if len(refused) != 1 {
		t.Fatalf("refused %d documents, and one was bad", len(refused))
	}
	// Named, because an operator reading the log has a store full of
	// definitions and needs to know which one to open.
	if !strings.Contains(refused[0].Error(), "typo-schedule") {
		t.Fatalf("the reason does not name the definition: %v", refused[0])
	}

	if got := len(r.Reports()); got != 1 {
		t.Fatalf("kept %d reports, and one was perfectly good", got)
	}
	if got := len(r.Schedules()); got != 0 {
		t.Fatalf("kept %d schedules, and the only one was refused", got)
	}
}

// The case that matters most: nothing wrong at all, and the fallback behaves
// exactly like the ordinary path.
func TestAdoptUsableRefusesNothingWhenNothingIsWrong(t *testing.T) {
	r := empty()

	if refused := r.AdoptUsable([][]byte{[]byte(goodReport)}); len(refused) != 0 {
		t.Fatalf("refused %v from a document that is fine", refused)
	}
	if got := len(r.Reports()); got != 1 {
		t.Fatalf("kept %d reports of one", got)
	}
}
