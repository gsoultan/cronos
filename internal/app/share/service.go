// Package share hands a report to somebody who is not in the project.
package share

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	core "github.com/gsoultan/cronos/internal/core/share"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// Store is where shares are recorded, so they can be listed and withdrawn.
type Store interface {
	Shared(ctx context.Context, pr principal.Principal, s core.Share) error
	Shares(ctx context.Context, pr principal.Principal) ([]core.Share, error)
	Share(ctx context.Context, id string) (core.Share, error)
	Revoke(ctx context.Context, pr principal.Principal, id string, at time.Time) error
}

// Reports is what a share is checked against before it is handed out.
//
// Both halves matter: that the report exists, and what it reads. A link to a
// report nobody has fails for the recipient, and a link to one whose rows are
// scoped per customer would hand every customer's rows to whoever holds the
// URL.
type Reports interface {
	Names(kind string) []string
	Report(ctx context.Context, name string) (definition.Report, error)
	Dataset(ctx context.Context, name string) (definition.Dataset, error)
}

// Service mints share links.
type Service struct {
	store  Store
	signer *token.Signer
	// reports resolves whose reports a share may name, from the sharer.
	//
	// A function rather than a value, because one process may serve several
	// projects: a link to a report that happens to exist in somebody else's is
	// a link to somebody else's data, and the check that prevents it is the
	// one that asks which project the sharer is in.
	reports func(principal.Principal) Reports
	now     func() time.Time
}

// New wires a Service over one project's reports.
func New(st Store, signer *token.Signer, reports Reports) *Service {
	return NewPerProject(st, signer, func(principal.Principal) Reports { return reports })
}

// NewPerProject wires a Service that resolves reports from the sharer.
func NewPerProject(st Store, signer *token.Signer,
	reports func(principal.Principal) Reports) *Service {

	return &Service{store: st, signer: signer, reports: reports, now: time.Now}
}

// WithClock makes the timestamps predictable in a test.
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// Request is what somebody asked to share.
type Request struct {
	Report string
	// Days until the link stops working. Zero means never, which the interface
	// offers and hiding would not prevent.
	Days int
	// Scope is the row constraint the recipient reads through.
	Scope map[string]string
}

// Create records a link to one report.
func (s *Service) Create(ctx context.Context, pr principal.Principal, req Request) (core.Share, error) {
	if !pr.CanEdit() {
		return core.Share{}, fmt.Errorf("%w: %s may not share", ErrForbidden, pr.ProjectRole)
	}
	if req.Report == "" {
		return core.Share{}, fmt.Errorf("%w: a share names one report", ErrInvalid)
	}
	// Checked here rather than at the first click. A link to a report that
	// does not exist fails for the recipient, who cannot tell whether they were
	// sent the wrong thing or refused.
	if !s.has(pr, req.Report) {
		return core.Share{}, fmt.Errorf("%w: no report %q", ErrInvalid, req.Report)
	}
	if req.Days < 0 {
		return core.Share{}, fmt.Errorf("%w: an expiry is days from now, or zero for never", ErrInvalid)
	}
	if err := s.scoped(ctx, pr, req); err != nil {
		return core.Share{}, err
	}

	now := s.now().UTC()
	sh := core.Share{
		ID:      newID(now),
		Org:     pr.OrgID,
		Project: pr.ProjectID,
		Report:  req.Report,
		// The sharer's own scope, copied. An author is a project member and
		// usually has none, in which case the link shows the project's rows —
		// which is what the person clicking Share was looking at.
		Scope:     req.Scope,
		CreatedBy: pr.Subject,
		CreatedAt: now,
	}
	if req.Days > 0 {
		until := now.AddDate(0, 0, req.Days)
		sh.ExpiresAt = &until
	}

	if err := s.store.Shared(ctx, pr, sh); err != nil {
		return core.Share{}, err
	}
	return sh, nil
}

/*
Open exchanges a share for a token that reads it.

The link somebody emails is the share's id, not a token, and this is why. An
embed token may live twenty-four hours at most — it ends up in a browser, and a
long-lived one is a permanent credential sitting in somebody's devtools. A
share is wanted for a week or a month, which those two facts cannot both be.

So the durable half is the record, which can be listed and withdrawn, and the
credential half is minted here, briefly, each time somebody opens the link.
Revoking is then a thing that happens rather than a thing that will have
happened once the last token expires.
*/
func (s *Service) Open(ctx context.Context, id string) (string, core.Share, error) {
	sh, err := s.store.Share(ctx, id)
	if err != nil {
		// The same answer for a share that never existed and one withdrawn an
		// hour ago. Telling them apart would let somebody with a dead link
		// learn that a live one had been there.
		return "", core.Share{}, fmt.Errorf("%w: %s", ErrNotOpen, id)
	}
	if !sh.Live(s.now().UTC()) {
		return "", core.Share{}, fmt.Errorf("%w: %s", ErrNotOpen, id)
	}

	raw, err := s.signer.Mint(token.Claims{
		// Embed, never portal. The recipient is an end reader of one report,
		// and a portal token would let them list the project and publish to it.
		Audience: token.Embed,
		Org:      sh.Org,
		Project:  sh.Project,
		// Traceable to the share rather than to a person: the recipient is not
		// somebody cronos has ever heard of.
		Subject: "share:" + sh.ID,
		Report:  sh.Report,
		Scope:   sh.Scope,
		// Carried so that revoking takes effect on the next request rather
		// than whenever this token happens to expire.
		ID: sh.ID,
	}, token.MaxLifetime)
	if err != nil {
		return "", core.Share{}, err
	}
	return raw, sh, nil
}

// List returns what this project has handed out.
func (s *Service) List(ctx context.Context, pr principal.Principal) ([]core.Share, error) {
	return s.store.Shares(ctx, pr)
}

// Revoke withdraws one.
func (s *Service) Revoke(ctx context.Context, pr principal.Principal, id string) error {
	if !pr.CanEdit() {
		return fmt.Errorf("%w: %s may not revoke a share", ErrForbidden, pr.ProjectRole)
	}
	return s.store.Revoke(ctx, pr, id, s.now().UTC())
}

// Allows reports whether a token bearing this id may still be used.
//
// The half a signature cannot do. A token is valid until it expires and
// nothing about the signature can be taken back, so every request carrying one
// asks this — and a share that is missing, revoked, expired, or recorded
// against a different project than the token claims is refused.
func (s *Service) Allows(ctx context.Context, id, org, project string) bool {
	if id == "" {
		// A token we never issued. Hosts mint their own for their own users,
		// and those are governed by their expiry alone — there is nothing here
		// to revoke and pretending otherwise would refuse every one of them.
		return true
	}
	sh, err := s.store.Share(ctx, id)
	if err != nil {
		return false
	}
	if sh.Org != org || sh.Project != project {
		return false
	}
	return sh.Live(s.now().UTC())
}

/*
scoped refuses a link to a report whose rows belong to somebody in particular.

The recipient of a share holds an embed token, and docs/tenancy.md is explicit
about what that means: row scope applies to them, and a dataset referencing
{{ .scope.x }} cannot be read without that scope. So a share of such a report
either shows nothing, or — if we quietly marked the token as a project member —
shows every customer's rows to whoever the URL reaches.

Neither is a link worth handing out, and the second is the failure row scope
exists to prevent. So it is refused, and the message says whose rows the link
would have to name. Sharing one of these means saying which customer it is for,
which is a question only the person sharing can answer.
*/
func (s *Service) scoped(ctx context.Context, pr principal.Principal, req Request) error {
	// A sharer with a scope of their own is already narrowed to it, and the
	// token carries that same narrowing to the recipient.
	reports := s.reports(pr)
	if len(req.Scope) > 0 || reports == nil {
		return nil
	}

	rep, err := reports.Report(ctx, req.Report)
	if err != nil {
		return nil // the report exists; has() said so, and this is not that check
	}
	for _, name := range datasets(rep) {
		ds, err := reports.Dataset(ctx, name)
		if err != nil {
			// Fails closed. A dataset we cannot read is one we cannot say is
			// safe to publish.
			return fmt.Errorf("%w: cannot tell whether %q is scoped per customer", ErrInvalid, name)
		}
		if len(ds.RowLevelSecurity) > 0 {
			return fmt.Errorf(
				"%w: %q reads %q, whose rows belong to one customer at a time — "+
					"a link to it would have to say which", ErrScoped, req.Report, name)
		}
	}
	return nil
}

// datasets is every dataset a report reads: its default and any a block
// overrides it with.
func datasets(rep definition.Report) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	add(rep.Dataset)
	for _, o := range rep.Outputs {
		for _, b := range o.Layout {
			add(b.Dataset)
		}
	}
	return out
}

func (s *Service) has(pr principal.Principal, report string) bool {
	reports := s.reports(pr)
	if reports == nil {
		return true
	}
	for _, name := range reports.Names("Report") {
		if name == report {
			return true
		}
	}
	return false
}

// newID names a share.
//
// Long enough that it is not guessable, because knowing one is not permission
// to do anything with it but is a fact about the project nobody granted.
func newID(at time.Time) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice, and a share that could not be
		// named must not be a share that was handed out unrevokable.
		return fmt.Sprintf("shr_%d_unnamed", at.UTC().Unix())
	}
	return "shr_" + hex.EncodeToString(b[:])
}
