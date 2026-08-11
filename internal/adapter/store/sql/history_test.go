package sql_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/history"
)

func aRun(id string) history.Run {
	return history.Run{
		ID: id, Org: "acme", Project: "finance",
		Schedule: "monthly-statements", Report: "customer-statement",
		ReportVersion: "sha256:abcdef123456", Output: "pdf",
		PeriodStart: "2026-07-01", PeriodEnd: "2026-08-01",
		TriggeredBy: "schedule:monthly-statements",
		StartedAt:   time.Date(2026, 8, 1, 4, 0, 0, 0, time.UTC),
		Status:      history.Running,
	}
}

// The question the table exists for: what did this customer receive, and
// against which version of the definition.
func TestARunAndItsDeliveriesComeBack(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	run := aRun("run_1_aaaa")
	if err := s.Begin(ctx, run); err != nil {
		t.Fatal(err)
	}
	for _, d := range []history.Delivery{
		{RunID: run.ID, Recipient: "c-1", Channel: "email", Destination: "a@b.example",
			Filename: "statement-c-1.pdf", Status: history.Delivered, Attempts: 1,
			Bytes: 24000, At: run.StartedAt},
		{RunID: run.ID, Recipient: "c-2", Channel: "email", Destination: "x@y.example",
			Status: history.Failed, Attempts: 4, Error: "after 4 attempts: 550 no mailbox",
			At: run.StartedAt},
	} {
		if err := s.Delivered(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	done := run
	at := run.StartedAt.Add(90 * time.Second)
	done.FinishedAt, done.Recipients, done.Delivered, done.Status =
		&at, 2, 1, history.Partial

	if err := s.Finish(ctx, run.ID, done); err != nil {
		t.Fatal(err)
	}

	got, deliveries, err := s.Run(ctx, acme, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The version is the point: a run naming one can be replayed against
	// exactly the document that produced it.
	if got.ReportVersion != "sha256:abcdef123456" {
		t.Errorf("version = %q", got.ReportVersion)
	}
	if got.Status != history.Partial || got.Delivered != 1 {
		t.Errorf("run = %+v", got)
	}
	if got.Took() != 90*time.Second {
		t.Errorf("took %s", got.Took())
	}
	if len(deliveries) != 2 {
		t.Fatalf("got %d deliveries", len(deliveries))
	}
	// "We emailed them" is not an answer to an auditor; the address is.
	if deliveries[0].Destination != "a@b.example" {
		t.Errorf("destination = %q", deliveries[0].Destination)
	}
	if deliveries[1].Attempts != 4 || deliveries[1].Status != history.Failed {
		t.Errorf("the failed one = %+v", deliveries[1])
	}
}

// A burst that crashed halfway is exactly the run somebody needs to look at.
func TestAnUnfinishedRunIsStillARun(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	if err := s.Begin(ctx, aRun("run_2_bbbb")); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Run(ctx, acme, "run_2_bbbb")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != history.Running || got.FinishedAt != nil {
		t.Errorf("run = %+v", got)
	}
	if got.Took() != 0 {
		t.Errorf("an unfinished run took %s", got.Took())
	}
}

// A retry is the same delivery having another go, not a second one. Two rows
// would make a burst look like it sent twice.
func TestARetryUpdatesTheDeliveryRatherThanAddingOne(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	run := aRun("run_3_cccc")
	if err := s.Begin(ctx, run); err != nil {
		t.Fatal(err)
	}

	first := history.Delivery{RunID: run.ID, Recipient: "c-1", Channel: "email",
		Status: history.Failed, Attempts: 1, At: run.StartedAt}
	if err := s.Delivered(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Status, second.Attempts = history.Delivered, 3
	if err := s.Delivered(ctx, second); err != nil {
		t.Fatal(err)
	}

	_, deliveries, err := s.Run(ctx, acme, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("got %d rows for one delivery", len(deliveries))
	}
	if deliveries[0].Attempts != 3 || deliveries[0].Status != history.Delivered {
		t.Errorf("delivery = %+v", deliveries[0])
	}
}

// A run id from another project must read as absent, or the difference tells a
// caller it exists.
func TestRunsAreScopedToTheirProject(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if err := s.Begin(ctx, aRun("run_4_dddd")); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.Run(ctx, who("northwind", "finance"), "run_4_dddd"); !errors.Is(err, publish.ErrNotFound) {
		t.Errorf("another tenant read it: %v", err)
	}
	if list, _ := s.Runs(ctx, who("acme", "operations"), 0); len(list) != 0 {
		t.Errorf("a sibling project listed %d runs", len(list))
	}
}

func TestRunsAreListedNewestFirstAndCapped(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	base := time.Date(2026, 8, 1, 4, 0, 0, 0, time.UTC)
	for i := range 5 {
		r := aRun("run_" + string(rune('a'+i)))
		r.StartedAt = base.Add(time.Duration(i) * time.Hour)
		if err := s.Begin(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	list, err := s.Runs(ctx, acme, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d runs, want the limit of 3", len(list))
	}
	if !list[0].StartedAt.After(list[1].StartedAt) {
		t.Errorf("not newest first: %s then %s", list[0].StartedAt, list[1].StartedAt)
	}

	// The first thing anyone does with an unbounded listing is call it on a
	// table with a year of bursts in it.
	if all, _ := s.Runs(ctx, acme, 100000); len(all) > 500 {
		t.Errorf("returned %d rows", len(all))
	}
}
