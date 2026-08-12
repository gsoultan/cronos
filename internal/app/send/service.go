/*
Package send delivers one report, once, to people the sender names.

A schedule is the same act on a timer, and this is deliberately not built on
one: a schedule is a definition somebody publishes and can be found later, and
"email this to my colleague now" is not something anybody wants left in a
project's git repository. What the two share is everything below the decision —
the renderer and the channels — and that is what this reuses.

Rendered as the sender. The recipients are named by somebody who could already
see the report, and they receive that view rather than one computed for an
identity none of them have; a link is what to send when they should see their
own rows, and the share panel says so beside this.
*/
package send

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Reports resolves the report being sent.
type Reports interface {
	Report(ctx context.Context, name string) (definition.Report, error)
}

// Service sends a report.
type Service struct {
	reports   Reports
	documents burst.Documents
	channels  map[string]burst.Channel
	now       func() time.Time
}

// New wires a Service. Channels are indexed once, so resolving one is a lookup.
func New(r Reports, d burst.Documents, channels ...burst.Channel) *Service {
	byName := make(map[string]burst.Channel, len(channels))
	for _, c := range channels {
		byName[c.Name()] = c
	}
	return &Service{reports: r, documents: d, channels: byName, now: time.Now}
}

// WithClock makes the filename predictable in a test.
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// Request is what somebody asked to send.
type Request struct {
	Report string
	// Output names the profile to render: a PDF to attach, or a spreadsheet.
	Output string
	// Via is the channel — email, telegram, whatever is configured.
	Via string
	// To is everyone who should receive it.
	To      []string
	Subject string
	Note    string
}

// Result is what happened, per recipient.
type Result struct {
	Sent   []string          `json:"sent"`
	Failed map[string]string `json:"failed,omitempty"`
	Bytes  int               `json:"bytes"`
}

// Send renders once and delivers to everybody named.
//
// Once, because the document does not depend on who receives it — that is the
// difference between this and a burst, and rendering per recipient would be
// the same PDF typeset eight times.
func (s *Service) Send(ctx context.Context, req Request, pr principal.Principal) (Result, error) {
	if !pr.CanEdit() {
		return Result{}, fmt.Errorf("%w: %s may not send reports", ErrForbidden, pr.ProjectRole)
	}
	if len(req.To) == 0 {
		return Result{}, fmt.Errorf("%w: nobody to send it to", ErrInvalid)
	}
	if len(req.To) > MaxRecipients {
		// A bound, because this endpoint renders once and delivers many, and
		// the many is a number somebody typed. A schedule is what a thousand
		// recipients wants: it is recorded, resumable and rate-limited by its
		// own concurrency.
		return Result{}, fmt.Errorf("%w: %d recipients at once — publish a schedule for more than %d",
			ErrInvalid, len(req.To), MaxRecipients)
	}

	channel, ok := s.channels[req.Via]
	if !ok {
		return Result{}, fmt.Errorf("%w: no %q channel is configured", ErrInvalid, req.Via)
	}

	report, err := s.reports.Report(ctx, req.Report)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}

	rendered, err := s.documents.Statement(ctx, report, req.Output, nil, pr)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrRender, err)
	}

	result := Result{Failed: map[string]string{}, Bytes: len(rendered.Document)}
	filename := s.filename(report, req.Output)

	for _, to := range req.To {
		err := channel.Deliver(ctx, burst.Delivery{
			To:       to,
			Filename: filename,
			Subject:  s.subject(req, report),
			Body:     req.Note,
			Document: rendered.Document,
		})
		if err != nil {
			// One address failing does not stop the rest. A send to eight
			// people where the second has a typo should reach the other seven,
			// and say which one it did not.
			result.Failed[to] = err.Error()
			continue
		}
		result.Sent = append(result.Sent, to)
	}
	return result, nil
}

// MaxRecipients bounds one send.
const MaxRecipients = 50

func (s *Service) subject(req Request, report definition.Report) string {
	if strings.TrimSpace(req.Subject) != "" {
		return req.Subject
	}
	return report.Heading()
}

// filename is what lands in somebody's downloads folder.
//
// Dated, because a folder of files all called statement.pdf is a folder nobody
// can search — the same reason a burst resolves one per recipient.
func (s *Service) filename(report definition.Report, output string) string {
	name := report.Name
	if name == "" {
		name = "report"
	}
	return fmt.Sprintf("%s-%s.%s", name, s.now().UTC().Format("2006-01-02"), extension(output))
}

func extension(output string) string {
	switch output {
	case "xlsx", "excel", "spreadsheet":
		return "xlsx"
	case "csv":
		return "csv"
	default:
		return "pdf"
	}
}
