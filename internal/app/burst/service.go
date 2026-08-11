package burst

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/history"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Reports and Datasets resolve what a schedule names.
type Reports interface {
	Report(ctx context.Context, name string) (definition.Report, error)
}

// Versions resolves the content address of the definition that ran.
//
// Separate from Reports because it answers a different question: Reports says
// what to render, this says which bytes said so. A run record naming a report
// by name alone is reproducible only until somebody edits it.
type Versions interface {
	Version(kind, name string) (string, bool)
}

// Documents renders one recipient's document from a report.
//
// The mapping from a report's paginated layout to a document lives behind this
// port rather than in here, so a burst is about fan-out and delivery and not
// about what a statement looks like.
type Documents interface {
	Statement(ctx context.Context, r definition.Report, output string,
		params map[string]any, pr principal.Principal) (StatementResult, error)
}

// StatementResult is one rendered recipient.
type StatementResult struct {
	Document []byte
	// Rows is how many the statement covered. Zero is legitimate — a customer
	// billed nothing this period still gets a statement saying so — but the
	// schedule may want to know.
	Rows int
}

// Result is what a burst reports when it finishes.
type Result struct {
	// ID is the run record, so a caller can look up what went where.
	ID         string   `json:"id,omitempty"`
	Recipients int      `json:"recipients"`
	Delivered  int      `json:"delivered"`
	Failed     []string `json:"failed,omitempty"`
}

// Service runs bursts.
type Service struct {
	reports   Reports
	recipient Recipients
	documents Documents
	channels  map[string]Channel
	versions  Versions
	history   Recorder
	log       *slog.Logger
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
}

// WithVersions records which definition each run used.
//
// Optional, because a deployment can render without a store that addresses
// anything. Absent, the field is empty rather than a plausible guess: a run
// record naming the wrong version is worse than one naming none.
func (s *Service) WithVersions(v Versions) *Service {
	s.versions = v
	return s
}

// versionOf is the content address of the report, or nothing.
func (s *Service) versionOf(report string) string {
	if s.versions == nil {
		return ""
	}
	v, _ := s.versions.Version("Report", report)
	return v
}

// Recipients reads the rows a burst fans out over.
type Recipients interface {
	Rows(ctx context.Context, dataset string, params map[string]any,
		pr principal.Principal) ([]Row, error)
}

// New wires a Service. Channels are indexed by name once, so resolving a
// schedule's `via` is a lookup rather than a scan per delivery.
func New(r Reports, rec Recipients, d Documents, log *slog.Logger, channels ...Channel) *Service {
	byName := make(map[string]Channel, len(channels))
	for _, c := range channels {
		byName[c.Name()] = c
	}
	return &Service{
		reports: r, recipient: rec, documents: d, channels: byName,
		history: discard{}, log: log, now: time.Now, sleep: pause,
	}
}

// WithHistory records what each run delivered.
func (b *Service) WithHistory(r Recorder) *Service { b.history = r; return b }

// WithClock and WithSleep make retry behaviour testable without waiting out a
// backoff ladder in real time.
func (b *Service) WithClock(now func() time.Time) *Service { b.now = now; return b }
func (b *Service) WithSleep(f func(context.Context, time.Duration) error) *Service {
	b.sleep = f
	return b
}

// Run executes s as pr, for the given run context.
func (b *Service) Run(ctx context.Context, s definition.Schedule, run Run,
	pr principal.Principal) (Result, error) {

	record := history.Run{
		ID: history.NewID(b.now()), Org: pr.OrgID, Project: pr.ProjectID,
		Schedule: s.Name, Report: s.Report, Output: s.Output,
		ReportVersion: b.versionOf(s.Report),
		PeriodStart:   run["periodStart"], PeriodEnd: run["periodEnd"],
		TriggeredBy: pr.Subject, StartedAt: b.now(), Status: history.Running,
	}
	// Written before anything runs. A burst that crashed halfway is exactly the
	// run somebody needs to look at, and one recorded only on success is a log
	// of successes.
	b.record(ctx, record)

	report, err := b.reports.Report(ctx, s.Report)
	if err != nil {
		return Result{}, b.abandon(ctx, record, err)
	}
	if !s.Bursts() {
		// One document for everybody. Still a burst of one, so the same
		// rendering and delivery path runs — a second code path here is a
		// second place for delivery to be subtly different.
		return b.finish(ctx, record, b.fan(ctx, s, report, []Row{{}}, run, record.ID, pr)), nil
	}

	rows, err := b.recipient.Rows(ctx, s.Burst.Over.Dataset, s.Params, pr)
	if err != nil {
		return Result{}, b.abandon(ctx, record, err)
	}
	if len(rows) == 0 {
		return Result{}, b.abandon(ctx, record,
			fmt.Errorf("%w: %q returned no rows", ErrNoRecipients, s.Burst.Over.Dataset))
	}
	return b.finish(ctx, record, b.fan(ctx, s, report, rows, run, record.ID, pr)), nil
}

// record writes a run, and never fails a burst for it.
//
// A document that reached a customer has reached them whatever the audit table
// thinks. Unwinding that because a write failed would be the worse outcome.
func (b *Service) record(ctx context.Context, r history.Run) {
	if err := b.history.Begin(ctx, r); err != nil {
		b.log.Error("run not recorded", "run", r.ID, "schedule", r.Schedule, "err", err)
	}
}

// abandon closes a run that never got as far as delivering.
func (b *Service) abandon(ctx context.Context, r history.Run, cause error) error {
	at := b.now()
	r.FinishedAt, r.Status, r.Error = &at, history.Failed, cause.Error()
	if err := b.history.Finish(ctx, r.ID, r); err != nil {
		b.log.Error("run not recorded", "run", r.ID, "err", err)
	}
	return cause
}

// finish closes a run with what it managed.
func (b *Service) finish(ctx context.Context, r history.Run, result Result) Result {
	at := b.now()
	r.FinishedAt = &at
	r.Recipients, r.Delivered = result.Recipients, result.Delivered

	switch {
	case len(result.Failed) == 0:
		r.Status = history.Delivered
	case result.Delivered == 0:
		r.Status = history.Failed
	default:
		// Its own status, not a success with a footnote: 4,997 of 5,000 has
		// three customers without an invoice and something has to find them.
		r.Status = history.Partial
	}
	if err := b.history.Finish(ctx, r.ID, r); err != nil {
		b.log.Error("run not recorded", "run", r.ID, "err", err)
	}
	result.ID = r.ID
	return result
}

// fan renders and delivers every row, bounded.
//
// Bounded because the point of a burst is that it does not need a browser
// farm: five thousand recipients eight at a time is a steady process, and five
// thousand at once is a machine that stops.
func (b *Service) fan(ctx context.Context, s definition.Schedule, report definition.Report,
	rows []Row, run Run, runID string, pr principal.Principal) Result {

	workers := definition.DefaultConcurrency
	if s.Burst != nil {
		workers = s.Burst.Workers()
	}

	var (
		mu     sync.Mutex
		result = Result{Recipients: len(rows)}
		sem    = make(chan struct{}, workers)
		wg     sync.WaitGroup
	)

	for i, row := range rows {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, row Row) {
			defer wg.Done()
			defer func() { <-sem }()

			err := b.one(ctx, s, report, row, run, runID, pr)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				// One recipient's failure does not stop the other four
				// thousand nine hundred and ninety-nine. It is collected and
				// reported, because a partial burst that claimed success is
				// how a customer finds out by not receiving an invoice.
				b.log.Error("burst recipient failed", "schedule", s.Name, "row", i, "err", err)
				result.Failed = append(result.Failed, err.Error())
				return
			}
			result.Delivered++
		}(i, row)
	}
	wg.Wait()
	return result
}

// one renders and delivers a single recipient.
func (b *Service) one(ctx context.Context, s definition.Schedule, report definition.Report,
	row Row, run Run, runID string, pr principal.Principal) error {

	params, err := b.bind(s, row, run)
	if err != nil {
		return err
	}

	rendered, err := b.documents.Statement(ctx, report, s.Output, params, pr)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	for _, spec := range s.Deliver {
		if err := b.deliver(ctx, s, spec, rendered.Document, row, run, runID); err != nil {
			return err
		}
	}
	return nil
}

// bind resolves the schedule's parameters for this row.
func (b *Service) bind(s definition.Schedule, row Row, run Run) (map[string]any, error) {
	params := map[string]any{}
	for k, v := range s.Params {
		params[k] = v
	}
	if s.Burst == nil {
		return params, nil
	}
	for name, tmpl := range s.Burst.Bind {
		value, err := resolve(tmpl, row, run)
		if err != nil {
			return nil, fmt.Errorf("binding %q: %w", name, err)
		}
		params[name] = value
	}
	return params, nil
}

func (b *Service) deliver(ctx context.Context, s definition.Schedule, spec definition.DeliverSpec,
	doc []byte, row Row, run Run, runID string) error {

	channel, ok := b.channels[spec.Via]
	if !ok {
		// Not retried: a channel nobody registered will not appear during the
		// backoff.
		return Fatal(fmt.Errorf("no channel named %q", spec.Via))
	}

	fields, err := resolveAll(map[string]string{
		"to":       spec.To,
		"subject":  spec.Subject,
		"filename": spec.Attach.Filename,
		"body":     spec.Body.Text,
	}, row, run)
	if err != nil {
		return Fatal(err)
	}

	d := Delivery{
		To: fields["to"], Subject: fields["subject"],
		Filename: fields["filename"], Body: fields["body"],
		Document: bytes.Clone(doc), Options: spec.Options,
	}

	attempts, err := attempt(ctx, s.OnFailure, b.sleep, func() error {
		return channel.Deliver(ctx, d)
	})

	b.recordDelivery(ctx, runID, spec.Via, d, attempts, err)
	return err
}

// recordDelivery writes one row per recipient per channel.
//
// Both outcomes, not just failures: "we emailed them" is not an answer to an
// auditor and the address is, so the successful ones are the record and the
// failures are the exception within it.
func (b *Service) recordDelivery(ctx context.Context, runID, channel string,
	d Delivery, attempts int, cause error) {

	record := history.Delivery{
		RunID: runID, Recipient: d.To, Channel: channel,
		Destination: d.To, Filename: d.Filename,
		Attempts: attempts, Bytes: len(d.Document),
		Status: history.Delivered, At: b.now(),
	}
	if cause != nil {
		record.Status, record.Error = history.Failed, cause.Error()
	}
	if err := b.history.Delivered(ctx, record); err != nil {
		b.log.Error("delivery not recorded", "run", runID, "to", d.To, "err", err)
	}
}

func resolveAll(in map[string]string, row Row, run Run) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for key, tmpl := range in {
		v, err := resolve(tmpl, row, run)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = v
	}
	return out, nil
}
