package audit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/cronos/ee/audit"
	"github.com/gsoultan/cronos/internal/extension"
)

/*
The durable audit sink, and the one property it exists for.

Writes are serialized. A burst runs report renders concurrently and every one
of them can record, so without the lock two events interleave and the file
holds a line that is neither of them — which is the one failure an append-only
audit log cannot recover from, because there is nothing to reconcile against.

Exercised under -race, which is what the concurrent test is for: the assertion
is both that every line parses and that the detector sees no unsynchronised
write.
*/

func event(n string) extension.Event {
	return extension.Event{
		At:        time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC),
		Actor:     n,
		OrgID:     "acme",
		ProjectID: "finance",
		Action:    "report.read",
		Target:    "billing-summary",
		Result:    "allowed",
		Detail:    map[string]any{"rows": 42},
	}
}

func TestAnEventIsOneJSONObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	s := audit.NewJSONL(&buf)

	if err := s.Record(context.Background(), event("u1")); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("one event produced %d lines", len(lines))
	}

	var got extension.Event
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("the line is not JSON: %v — %s", err, lines[0])
	}
	if got.Actor != "u1" || got.Action != "report.read" || got.Target != "billing-summary" {
		t.Errorf("the event came back as %+v", got)
	}
	if got.Result != "allowed" {
		t.Errorf("result is %q, want allowed", got.Result)
	}
}

/*
Concurrent records do not interleave.

This is the whole reason the type holds a mutex, and the assertion is that
every line is a whole event — a partial one would parse as nothing, or worse,
as a different event than either writer wrote.
*/
func TestConcurrentRecordsEachProduceAWholeLine(t *testing.T) {
	var buf bytes.Buffer
	s := audit.NewJSONL(&buf)

	const writers, each = 8, 25

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				if err := s.Record(context.Background(), event(string(rune('a'+w)))); err != nil {
					t.Errorf("record: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != writers*each {
		t.Fatalf("wrote %d lines, want %d", len(lines), writers*each)
	}
	for i, line := range lines {
		var got extension.Event
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d is not a whole event: %v — %s", i, err, line)
		}
		if got.Target != "billing-summary" {
			t.Fatalf("line %d came back as %+v", i, got)
		}
	}
}

// A cancelled context is refused rather than written. Unlike the log sink,
// this one has a caller that can retry: the error says the event was not
// durably recorded, which is a true statement worth making.
func TestACancelledContextIsReportedRatherThanSwallowed(t *testing.T) {
	var buf bytes.Buffer
	s := audit.NewJSONL(&buf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Record(ctx, event("u1")); err == nil {
		t.Fatal("a cancelled context recorded silently")
	}
	if buf.Len() != 0 {
		t.Errorf("something was written anyway: %s", buf.String())
	}
}

// A writer that fails says so, naming the action, because the caller's next
// decision is whether the operation may proceed unaudited.
func TestAFailedWriteIsReportedAndNamesTheAction(t *testing.T) {
	s := audit.NewJSONL(broken{})

	err := s.Record(context.Background(), event("u1"))
	if err == nil {
		t.Fatal("a failed write was reported as success")
	}
	if !strings.Contains(err.Error(), "report.read") {
		t.Errorf("the error should name the action: %v", err)
	}
}

type broken struct{}

func (broken) Write([]byte) (int, error) { return 0, errBroken }

var errBroken = errBrokenType{}

type errBrokenType struct{}

func (errBrokenType) Error() string { return "the disk is full" }

func TestTheSinkIsNamedJSONL(t *testing.T) {
	if got := audit.NewJSONL(&bytes.Buffer{}).Name(); got != "jsonl" {
		t.Errorf("Name is %q, want jsonl", got)
	}
}

var _ extension.AuditSink = (*audit.JSONL)(nil)
