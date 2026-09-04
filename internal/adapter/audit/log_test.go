package audit_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/audit"
	"github.com/gsoultan/cronos/internal/extension"
)

/*
The audit trail that ships by default.

The package had no tests. What it has to get right is not the writing but the
shape: one line per event, at INFO, under a fixed message so a pipeline can
select on it, with the detail flattened rather than nested — a nested map
arrives as one opaque string in most log pipelines, which is the same as not
recording the fields at all.

And it never returns an error. A logger that cannot write has already failed in
a way this cannot fix, and an error here would make every call site log about
being unable to log.
*/

func sink() (*audit.Log, *bytes.Buffer) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return audit.NewLog(log), &buf
}

func event() extension.Event {
	return extension.Event{
		At:        time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC),
		Actor:     "u1",
		OrgID:     "acme",
		ProjectID: "finance",
		Action:    "report.read",
		Target:    "billing-summary",
		Result:    "allowed",
		Detail:    map[string]any{"rows": 42, "output": "pdf"},
	}
}

func TestAnEventIsOneLineUnderAFixedMessage(t *testing.T) {
	s, buf := sink()

	if err := s.Record(context.Background(), event()); err != nil {
		t.Fatalf("recording returned %v, and it never should", err)
	}

	out := buf.String()
	if lines := strings.Count(strings.TrimSpace(out), "\n"); lines != 0 {
		t.Errorf("one event produced %d lines, want one", lines+1)
	}
	// The fixed message is how a pipeline selects audit lines out of
	// everything else the process says.
	if !strings.Contains(out, `msg=audit`) {
		t.Errorf("the line is not selectable by its message: %s", out)
	}
	if !strings.Contains(out, "level=INFO") {
		t.Errorf("the line is not at INFO: %s", out)
	}
}

func TestEveryFieldAnAuditorAsksForIsOnTheLine(t *testing.T) {
	s, buf := sink()

	if err := s.Record(context.Background(), event()); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{
		"actor=u1", "org=acme", "project=finance",
		"action=report.read", "target=billing-summary", "result=allowed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the line is missing %s: %s", want, out)
		}
	}
}

/*
Detail is flattened, not nested.

A nested map reaches most log pipelines as one opaque string, so a field
recorded that way cannot be indexed or filtered on — which is the same as not
recording it, while looking like it was recorded.
*/
func TestDetailIsFlattenedSoAPipelineCanIndexIt(t *testing.T) {
	s, buf := sink()

	if err := s.Record(context.Background(), event()); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{"detail.rows=42", "detail.output=pdf"} {
		if !strings.Contains(out, want) {
			t.Errorf("the line is missing %s: %s", want, out)
		}
	}
}

// An event with nothing extra still records. Detail is optional and a nil map
// is the ordinary case for a refusal.
func TestAnEventWithNoDetailStillRecords(t *testing.T) {
	s, buf := sink()

	e := event()
	e.Detail = nil
	e.Result = "refused"

	if err := s.Record(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "result=refused") {
		t.Errorf("a refusal was not recorded: %s", buf.String())
	}
}

/*
A cancelled context does not stop an audit line.

The event has already happened. Declining to record it because the request it
belongs to was cancelled would lose exactly the entries an incident is
reconstructed from — somebody's request being cut off is not a reason to have
no record that it was made.
*/
func TestACancelledContextStillRecords(t *testing.T) {
	s, buf := sink()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Record(ctx, event()); err != nil {
		t.Fatalf("recording under a cancelled context returned %v", err)
	}
	if buf.Len() == 0 {
		t.Error("nothing was recorded for an event that had already happened")
	}
}

// The name appears in the startup line so an operator can see which sink is
// installed without reading the configuration back.
func TestTheSinkIsNamedLog(t *testing.T) {
	s, _ := sink()
	if got := s.Name(); got != "log" {
		t.Errorf("Name is %q, want log", got)
	}
}

// It satisfies the seam it is registered against.
var _ extension.AuditSink = (*audit.Log)(nil)
