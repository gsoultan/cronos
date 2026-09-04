package send_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/gsoultan/cronos/internal/app/send"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Sending one report to people the sender names.

The package had no tests. Three of the things it does are the kind that are
only noticed when they are wrong in production: it renders once for everybody
rather than once each, one bad address does not cost the other seven their
copy, and the recipient count is bounded because it is a number somebody typed
into a box.

Rendered as the sender, deliberately. Everyone named receives the sender's view
rather than one computed for an identity they do not have — a link is what to
send when they should see their own rows.
*/

const day = "2026-09-04"

var clock = func() time.Time { return time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC) }

/* -- fakes ---------------------------------------------------------------- */

type reports map[string]definition.Report

func (r reports) Report(_ context.Context, name string) (definition.Report, error) {
	rep, ok := r[name]
	if !ok {
		return definition.Report{}, errors.New("no such report")
	}
	return rep, nil
}

// renderer counts how many times a document was produced, which is the whole
// point of Send existing beside burst.
type renderer struct {
	calls int
	fail  bool
}

func (d *renderer) Statement(_ context.Context, _ definition.Report, _ string,
	_ map[string]any, _ principal.Principal) (burst.StatementResult, error) {

	d.calls++
	if d.fail {
		return burst.StatementResult{}, errors.New("the typesetter fell over")
	}
	return burst.StatementResult{Document: []byte("%PDF-1.7 pretend")}, nil
}

// post records what it was handed, and refuses anybody in refuse.
type post struct {
	name   string
	got    []burst.Delivery
	refuse map[string]bool
}

func (p *post) Name() string { return p.name }

func (p *post) Deliver(_ context.Context, d burst.Delivery) error {
	if p.refuse[d.To] {
		return errors.New("no such mailbox")
	}
	p.got = append(p.got, d)
	return nil
}

/* -- fixtures -------------------------------------------------------------- */

func service(t *testing.T, r *renderer, channels ...burst.Channel) *send.Service {
	t.Helper()
	return send.New(
		reports{"billing": {Name: "billing", Title: "Billing summary"}},
		r, channels...,
	).WithClock(clock)
}

func author() principal.Principal {
	return principal.Principal{
		Subject: "u1", OrgID: "acme", ProjectID: "finance",
		ProjectRole: principal.ProjectEditor,
	}
}

func viewer() principal.Principal {
	p := author()
	p.ProjectRole = principal.ProjectViewer
	return p
}

func request(to ...string) send.Request {
	return send.Request{Report: "billing", Output: "pdf", Via: "email", To: to}
}

/* -- refusals -------------------------------------------------------------- */

func TestAViewerMayNotSend(t *testing.T) {
	s := service(t, &renderer{}, &post{name: "email"})

	_, err := s.Send(context.Background(), request("a@b.example"), viewer())
	if !errors.Is(err, send.ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
}

func TestASendNeedsSomebodyToSendTo(t *testing.T) {
	s := service(t, &renderer{}, &post{name: "email"})

	_, err := s.Send(context.Background(), request(), author())
	if !errors.Is(err, send.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

/*
The recipient count is bounded, because it is a number somebody typed.

This endpoint renders once and delivers many, and the many is a text box. A
thousand recipients is what a schedule is for: recorded, resumable, and
rate-limited by its own concurrency.
*/
func TestTooManyRecipientsAreRefusedAndPointedAtASchedule(t *testing.T) {
	s := service(t, &renderer{}, &post{name: "email"})

	many := make([]string, send.MaxRecipients+1)
	for i := range many {
		many[i] = "person@example.test"
	}

	_, err := s.Send(context.Background(), request(many...), author())
	if !errors.Is(err, send.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "schedule") {
		t.Errorf("the message should say what to do instead: %v", err)
	}
}

// Exactly the maximum is allowed. The bound is a maximum, not a maximum minus
// one.
func TestExactlyTheMaximumNumberOfRecipientsIsAllowed(t *testing.T) {
	channel := &post{name: "email"}
	s := service(t, &renderer{}, channel)

	many := make([]string, send.MaxRecipients)
	for i := range many {
		many[i] = "person@example.test"
	}

	result, err := s.Send(context.Background(), request(many...), author())
	if err != nil {
		t.Fatalf("exactly the maximum was refused: %v", err)
	}
	if len(result.Sent) != send.MaxRecipients {
		t.Errorf("sent %d, want %d", len(result.Sent), send.MaxRecipients)
	}
}

func TestAChannelNobodyConfiguredIsRefusedBeforeAnythingIsRendered(t *testing.T) {
	r := &renderer{}
	s := service(t, r, &post{name: "email"})

	req := request("a@b.example")
	req.Via = "telegram"

	_, err := s.Send(context.Background(), req, author())
	if !errors.Is(err, send.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
	// And nothing was typeset for a send that could never leave.
	if r.calls != 0 {
		t.Errorf("rendered %d times for an unconfigured channel", r.calls)
	}
}

func TestARenderThatFailsSendsNobodyAnything(t *testing.T) {
	channel := &post{name: "email"}
	s := service(t, &renderer{fail: true}, channel)

	_, err := s.Send(context.Background(), request("a@b.example"), author())
	if !errors.Is(err, send.ErrRender) {
		t.Fatalf("got %v, want ErrRender", err)
	}
	if len(channel.got) != 0 {
		t.Errorf("%d deliveries went out after the render failed", len(channel.got))
	}
}

/* -- the send itself ------------------------------------------------------- */

/*
Rendered once, however many people are named.

This is the difference between Send and a burst: the document does not depend
on who receives it, so typesetting it eight times is the same PDF eight times.
*/
func TestTheDocumentIsRenderedOnceForEverybody(t *testing.T) {
	r := &renderer{}
	channel := &post{name: "email"}
	s := service(t, r, channel)

	result, err := s.Send(context.Background(),
		request("a@x.example", "b@x.example", "c@x.example"), author())
	if err != nil {
		t.Fatal(err)
	}

	if r.calls != 1 {
		t.Errorf("rendered %d times for three recipients, want once", r.calls)
	}
	if len(result.Sent) != 3 {
		t.Errorf("sent to %d, want 3", len(result.Sent))
	}
	if result.Bytes == 0 {
		t.Error("the result reports no bytes for a document that was produced")
	}
}

/*
One bad address does not cost the others their copy.

A send to eight people where the second has a typo should reach the other
seven, and say which one it did not — the alternative is somebody retrying the
whole thing and the seven getting it twice.
*/
func TestOneFailedRecipientDoesNotStopTheRest(t *testing.T) {
	channel := &post{name: "email", refuse: map[string]bool{"typo@x.example": true}}
	s := service(t, &renderer{}, channel)

	result, err := s.Send(context.Background(),
		request("a@x.example", "typo@x.example", "c@x.example"), author())
	if err != nil {
		t.Fatalf("one bad address failed the whole send: %v", err)
	}

	if len(result.Sent) != 2 {
		t.Errorf("sent to %v, want the two good addresses", result.Sent)
	}
	if _, named := result.Failed["typo@x.example"]; !named {
		t.Errorf("the failure does not say which address: %v", result.Failed)
	}
}

/* -- what the recipient sees ----------------------------------------------- */

// Dated, because a folder of files all called billing.pdf is a folder nobody
// can search.
func TestTheFilenameIsDatedAndMatchesTheOutput(t *testing.T) {
	for _, c := range []struct{ output, want string }{
		{"pdf", "billing-" + day + ".pdf"},
		{"xlsx", "billing-" + day + ".xlsx"},
		{"excel", "billing-" + day + ".xlsx"},
		{"spreadsheet", "billing-" + day + ".xlsx"},
		{"csv", "billing-" + day + ".csv"},
		// Anything else is a paginated document, which is the default output.
		{"statement", "billing-" + day + ".pdf"},
	} {
		t.Run(c.output, func(t *testing.T) {
			channel := &post{name: "email"}
			s := service(t, &renderer{}, channel)

			req := request("a@x.example")
			req.Output = c.output
			if _, err := s.Send(context.Background(), req, author()); err != nil {
				t.Fatal(err)
			}
			if got := channel.got[0].Filename; got != c.want {
				t.Errorf("filename is %q, want %q", got, c.want)
			}
		})
	}
}

func TestTheSubjectIsTheSendersOrTheReportsHeading(t *testing.T) {
	channel := &post{name: "email"}
	s := service(t, &renderer{}, channel)

	// Given one, it is used.
	req := request("a@x.example")
	req.Subject = "Your August statement"
	if _, err := s.Send(context.Background(), req, author()); err != nil {
		t.Fatal(err)
	}
	if got := channel.got[0].Subject; got != "Your August statement" {
		t.Errorf("subject is %q, want the one the sender wrote", got)
	}

	// Blank, and the report's own heading stands in rather than an empty
	// subject line.
	req.Subject = "   "
	if _, err := s.Send(context.Background(), req, author()); err != nil {
		t.Fatal(err)
	}
	if got := channel.got[1].Subject; got == "" || strings.TrimSpace(got) == "" {
		t.Errorf("a blank subject was sent as %q, want the report's heading", got)
	}
}
