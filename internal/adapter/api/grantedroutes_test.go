package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sendapp "github.com/gsoultan/cronos/internal/app/send"
	shareapp "github.com/gsoultan/cronos/internal/app/share"
	"github.com/gsoultan/cronos/internal/core/access"
	"github.com/gsoultan/cronos/internal/core/principal"
	coreshare "github.com/gsoultan/cronos/internal/core/share"
)

/*
Every route that reaches a report asks the same question.

The grant gate was wired to the read and the catalogue, and three other routes
reached the same report without it: sending renders the original and mails it,
sharing turns it into a link that opens without an account, and running a
schedule renders and delivers it on demand. A refusal on one path while the
data leaves through another is not a control.

These are handler-level rather than end-to-end because the bug they guard
against is a missing call, not a wrong decision — core/access is tested
separately, and what is asserted here is that each handler consults it at all.
That is exactly the mistake made once already: the share handler was given a
gate field and a WithGrants builder, and the call itself was never inserted, so
it compiled, passed every test, and shared a restricted report.
*/

// refusing is a Granting that restricts one report to a group nobody is in.
type refusing struct{}

func (refusing) Grants(context.Context, string, string) ([]access.Grant, error) {
	return []access.Grant{{Report: "payroll", Kind: access.KindGroup, Subject: "finance"}}, nil
}
func (refusing) GroupsOf(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}

// as authenticates every request as one principal. The external test package
// has an equivalent; this file is internal, so it needs its own.
type as struct{ pr principal.Principal }

func (a as) Principal(*http.Request) (principal.Principal, bool) {
	return a.pr, a.pr.Subject != ""
}

func anEditor() principal.Principal {
	return principal.Principal{
		Subject: "u-1", OrgID: "acme", ProjectID: "finance",
		ProjectRole: principal.ProjectEditor, Member: true,
	}
}

/* -- send ------------------------------------------------------------------ */

type sentTo struct{ called bool }

func (s *sentTo) Send(context.Context, sendapp.Request, principal.Principal) (sendapp.Result, error) {
	s.called = true
	return sendapp.Result{}, nil
}

func TestSendingARestrictedReportIsRefused(t *testing.T) {
	svc := &sentTo{}
	h := NewSend(svc, as{anEditor()}, silent()).WithGrants(refusing{})

	r := httptest.NewRequest(http.MethodPost, "/v1/reports/payroll/send",
		strings.NewReader(`{"output":"pdf","via":"email","to":["attacker@example.test"]}`))
	r.SetPathValue("name", "payroll")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("answered %d, want 404: %s", w.Code, w.Body.String())
	}
	// The service must not have run: a refusal that still rendered and mailed
	// the report is not a refusal.
	if svc.called {
		t.Error("the report was sent despite the refusal")
	}
}

// And an ungranted report still sends, or the feature is an outage.
func TestSendingAnUnrestrictedReportStillWorks(t *testing.T) {
	svc := &sentTo{}
	h := NewSend(svc, as{anEditor()}, silent()).WithGrants(refusing{})

	r := httptest.NewRequest(http.MethodPost, "/v1/reports/billing/send",
		strings.NewReader(`{"output":"pdf","via":"email","to":["someone@example.test"]}`))
	r.SetPathValue("name", "billing")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !svc.called {
		t.Fatalf("a report nobody restricted was not sent: %d %s", w.Code, w.Body.String())
	}
}

/* -- share ----------------------------------------------------------------- */

type shared struct{ called bool }

func (s *shared) Create(_ context.Context, _ principal.Principal, req shareapp.Request) (coreshare.Share, error) {
	s.called = true
	return coreshare.Share{ID: "shr_x", Report: req.Report}, nil
}
func (s *shared) List(context.Context, principal.Principal) ([]coreshare.Share, error) {
	return nil, nil
}
func (s *shared) Revoke(context.Context, principal.Principal, string) error { return nil }
func (s *shared) Open(context.Context, string) (string, coreshare.Share, error) {
	return "", coreshare.Share{}, nil
}

/*
The one that got away.

A share is a durable link that opens without an account — the only
unauthenticated route in the API — so creating one for a report the caller may
not open converts a refusal into anonymous access for anybody with the URL.
*/
func TestSharingARestrictedReportIsRefused(t *testing.T) {
	svc := &shared{}
	h := NewShares(svc, as{anEditor()}, silent()).WithGrants(refusing{})

	r := httptest.NewRequest(http.MethodPost, "/v1/shares",
		strings.NewReader(`{"report":"payroll","days":30}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("answered %d, want 404: %s", w.Code, w.Body.String())
	}
	if svc.called {
		t.Error("a share was created for a report the caller may not open")
	}
}

func TestSharingAnUnrestrictedReportStillWorks(t *testing.T) {
	svc := &shared{}
	h := NewShares(svc, as{anEditor()}, silent()).WithGrants(refusing{})

	r := httptest.NewRequest(http.MethodPost, "/v1/shares",
		strings.NewReader(`{"report":"billing","days":30}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !svc.called {
		t.Fatalf("a report nobody restricted could not be shared: %d %s", w.Code, w.Body.String())
	}
}

/*
An administrator is exempt on every one of these, the same as on the read.

They are who adds and removes grants, so a deployment where an admin cannot
send or share a report they administer has a recovery path that ends at a psql
prompt.
*/
func TestAnAdministratorIsExemptOnTheseRoutesToo(t *testing.T) {
	admin := anEditor()
	admin.ProjectRole = principal.ProjectAdmin

	send := &sentTo{}
	sh := NewSend(send, as{admin}, silent()).WithGrants(refusing{})
	r := httptest.NewRequest(http.MethodPost, "/v1/reports/payroll/send",
		strings.NewReader(`{"output":"pdf","via":"email","to":["boss@example.test"]}`))
	r.SetPathValue("name", "payroll")
	sh.ServeHTTP(httptest.NewRecorder(), r)
	if !send.called {
		t.Error("an administrator could not send a report they administer")
	}

	share := &shared{}
	NewShares(share, as{admin}, silent()).WithGrants(refusing{}).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/shares",
			strings.NewReader(`{"report":"payroll","days":30}`)))
	if !share.called {
		t.Error("an administrator could not share a report they administer")
	}
}
