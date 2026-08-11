package burst

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Reports and Datasets resolve what a schedule names.
type Reports interface {
	Report(ctx context.Context, name string) (definition.Report, error)
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
	log       *slog.Logger
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
	return &Service{reports: r, recipient: rec, documents: d, channels: byName, log: log}
}

// Run executes s as pr, for the given run context.
func (b *Service) Run(ctx context.Context, s definition.Schedule, run Run,
	pr principal.Principal) (Result, error) {

	report, err := b.reports.Report(ctx, s.Report)
	if err != nil {
		return Result{}, err
	}
	if !s.Bursts() {
		// One document for everybody. Still a burst of one, so the same
		// rendering and delivery path runs — a second code path here is a
		// second place for delivery to be subtly different.
		return b.fan(ctx, s, report, []Row{{}}, run, pr), nil
	}

	rows, err := b.recipient.Rows(ctx, s.Burst.Over.Dataset, s.Params, pr)
	if err != nil {
		return Result{}, err
	}
	if len(rows) == 0 {
		return Result{}, fmt.Errorf("%w: %q returned no rows", ErrNoRecipients, s.Burst.Over.Dataset)
	}
	return b.fan(ctx, s, report, rows, run, pr), nil
}

// fan renders and delivers every row, bounded.
//
// Bounded because the point of a burst is that it does not need a browser
// farm: five thousand recipients eight at a time is a steady process, and five
// thousand at once is a machine that stops.
func (b *Service) fan(ctx context.Context, s definition.Schedule, report definition.Report,
	rows []Row, run Run, pr principal.Principal) Result {

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

			err := b.one(ctx, s, report, row, run, pr)

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
	row Row, run Run, pr principal.Principal) error {

	params, err := b.bind(s, row, run)
	if err != nil {
		return err
	}

	rendered, err := b.documents.Statement(ctx, report, s.Output, params, pr)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	for _, spec := range s.Deliver {
		if err := b.deliver(ctx, spec, rendered.Document, row, run); err != nil {
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

func (b *Service) deliver(ctx context.Context, spec definition.DeliverSpec,
	doc []byte, row Row, run Run) error {

	channel, ok := b.channels[spec.Via]
	if !ok {
		return fmt.Errorf("no channel named %q", spec.Via)
	}

	fields, err := resolveAll(map[string]string{
		"to":       spec.To,
		"subject":  spec.Subject,
		"filename": spec.Attach.Filename,
		"body":     spec.Body.Text,
	}, row, run)
	if err != nil {
		return err
	}

	return channel.Deliver(ctx, Delivery{
		To: fields["to"], Subject: fields["subject"],
		Filename: fields["filename"], Body: fields["body"],
		Document: bytes.Clone(doc), Options: spec.Options,
	})
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
