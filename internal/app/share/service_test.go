package share_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/app/share"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	core "github.com/gsoultan/cronos/internal/core/share"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Handing out a link to somebody who has no account.

The package had no tests, and it is where the product's sharpest security
decision lives: a link is a credential that leaves the building, and the one
combination that must not exist is a live link running as the sender for
anybody holding the URL. docs/report-format.md calls that "the one shape that
turns row-level security into decoration". The refusal that prevents it is
scoped(), and the assertions below are that sentence turned into tests.

The rest is the revocation story. A signature cannot be taken back, so the
share id travels in the token and every request asks Allows — which means the
interesting cases are the ones where the record and the token disagree.
*/

const (
	org     = "acme"
	project = "finance"
)

/* -- fakes ---------------------------------------------------------------- */

type store struct {
	saved   map[string]core.Share
	revoked map[string]time.Time
}

func newStore() *store {
	return &store{saved: map[string]core.Share{}, revoked: map[string]time.Time{}}
}

func (s *store) Shared(_ context.Context, _ principal.Principal, sh core.Share) error {
	s.saved[sh.ID] = sh
	return nil
}

func (s *store) Shares(_ context.Context, pr principal.Principal) ([]core.Share, error) {
	var out []core.Share
	for _, sh := range s.saved {
		if sh.Org == pr.OrgID && sh.Project == pr.ProjectID {
			out = append(out, sh)
		}
	}
	return out, nil
}

func (s *store) Share(_ context.Context, id string) (core.Share, error) {
	sh, ok := s.saved[id]
	if !ok {
		return core.Share{}, errors.New("no such share")
	}
	if at, gone := s.revoked[id]; gone {
		sh.RevokedAt = &at
	}
	return sh, nil
}

func (s *store) Revoke(_ context.Context, _ principal.Principal, id string, at time.Time) error {
	s.revoked[id] = at
	return nil
}

// catalog is one project's reports and datasets.
type catalog struct {
	reports  map[string]definition.Report
	datasets map[string]definition.Dataset
	// unreadable names a dataset the catalogue cannot resolve, for the
	// fails-closed case.
	unreadable string
}

func (c catalog) Names(string) []string {
	out := make([]string, 0, len(c.reports))
	for name := range c.reports {
		out = append(out, name)
	}
	return out
}

func (c catalog) Report(_ context.Context, name string) (definition.Report, error) {
	rep, ok := c.reports[name]
	if !ok {
		return definition.Report{}, errors.New("no such report")
	}
	return rep, nil
}

func (c catalog) Dataset(_ context.Context, name string) (definition.Dataset, error) {
	if name == c.unreadable {
		return definition.Dataset{}, errors.New("cannot read it")
	}
	ds, ok := c.datasets[name]
	if !ok {
		return definition.Dataset{}, errors.New("no such dataset")
	}
	return ds, nil
}

/* -- fixtures -------------------------------------------------------------- */

func open() catalog {
	return catalog{
		reports:  map[string]definition.Report{"summary": {Name: "summary", Dataset: "totals"}},
		datasets: map[string]definition.Dataset{"totals": {Name: "totals"}},
	}
}

// perCustomer is the catalogue whose dataset belongs to one customer at a
// time, which is the whole point of the refusal.
func perCustomer() catalog {
	c := open()
	c.datasets["totals"] = definition.Dataset{
		Name:             "totals",
		RowLevelSecurity: []definition.RowScope{{Predicate: "customer_id = {{ .scope.customer_id }}"}},
	}
	return c
}

func service(t *testing.T, c catalog) (*share.Service, *store) {
	t.Helper()

	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	st := newStore()
	return share.New(st, signer, c), st
}

func author() principal.Principal {
	return principal.Principal{
		Subject: "u1", OrgID: org, ProjectID: project,
		ProjectRole: principal.ProjectEditor,
	}
}

func viewer() principal.Principal {
	p := author()
	p.ProjectRole = principal.ProjectViewer
	return p
}

func mustCreate(t *testing.T, s *share.Service, pr principal.Principal, req share.Request) core.Share {
	t.Helper()
	sh, err := s.Create(context.Background(), pr, req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return sh
}

/* -- creating -------------------------------------------------------------- */

func TestAViewerMayNotShare(t *testing.T) {
	s, _ := service(t, open())

	_, err := s.Create(context.Background(), viewer(), share.Request{Report: "summary"})
	if !errors.Is(err, share.ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
}

func TestAShareMustNameAReportThatExists(t *testing.T) {
	s, _ := service(t, open())

	for _, name := range []string{"", "ghost"} {
		_, err := s.Create(context.Background(), author(), share.Request{Report: name})
		if !errors.Is(err, share.ErrInvalid) {
			t.Errorf("report %q: got %v, want ErrInvalid", name, err)
		}
	}
}

func TestAnExpiryIsDaysFromNowOrNever(t *testing.T) {
	s, _ := service(t, open())

	_, err := s.Create(context.Background(), author(), share.Request{Report: "summary", Days: -1})
	if !errors.Is(err, share.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid for a negative expiry", err)
	}

	// Zero is never, which the interface offers and hiding would not prevent.
	sh := mustCreate(t, s, author(), share.Request{Report: "summary"})
	if sh.ExpiresAt != nil {
		t.Errorf("zero days gave an expiry of %s, want never", sh.ExpiresAt)
	}
}

/*
The refusal the whole design turns on.

A report whose rows belong to one customer at a time cannot be handed out on a
URL, because the link has no customer: whoever opens it would either see
nothing or see everybody, and the second is what row scope exists to prevent.
*/
func TestAReportWhoseRowsBelongToOneCustomerCannotBeSharedUnscoped(t *testing.T) {
	s, _ := service(t, perCustomer())

	_, err := s.Create(context.Background(), author(), share.Request{Report: "summary"})
	if !errors.Is(err, share.ErrScoped) {
		t.Fatalf("got %v, want ErrScoped", err)
	}
	// The message has to say which dataset, because the fix is to name a
	// customer and the author needs to know which report reads what.
	if !strings.Contains(err.Error(), "totals") {
		t.Errorf("the message should name the dataset: %v", err)
	}
}

// Said which customer, and it is allowed: the link is narrowed to them.
func TestTheSameReportSharesOnceTheLinkSaysWhichCustomer(t *testing.T) {
	s, _ := service(t, perCustomer())

	sh := mustCreate(t, s, author(), share.Request{
		Report: "summary", Scope: map[string]string{"customer_id": "c-1"},
	})
	if sh.Scope["customer_id"] != "c-1" {
		t.Fatalf("the share carries scope %v, want the one asked for", sh.Scope)
	}
}

// A dataset the catalogue cannot read is one nobody can say is safe, so it is
// refused rather than assumed open.
func TestADatasetThatCannotBeReadFailsClosed(t *testing.T) {
	c := perCustomer()
	c.unreadable = "totals"
	s, _ := service(t, c)

	_, err := s.Create(context.Background(), author(), share.Request{Report: "summary"})
	if !errors.Is(err, share.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid — an unreadable dataset must not pass", err)
	}
}

// Two shares never collide, and an id is not a counter somebody can walk.
func TestShareIdsAreUnguessableAndDistinct(t *testing.T) {
	s, _ := service(t, open())

	seen := map[string]bool{}
	for range 50 {
		id := mustCreate(t, s, author(), share.Request{Report: "summary"}).ID
		if seen[id] {
			t.Fatalf("id %q was issued twice", id)
		}
		if !strings.HasPrefix(id, "shr_") || len(id) < 16 {
			t.Fatalf("id %q is too short to be unguessable", id)
		}
		seen[id] = true
	}
}

/* -- opening --------------------------------------------------------------- */

/*
The token a link mints is an embed token and never a portal one.

The recipient is an end reader of one report. A portal token would let them
list the project's catalogue and publish to it, which is the difference between
sharing a report and handing over the account.
*/
func TestOpeningALinkMintsAnEmbedTokenAndNeverAPortalOne(t *testing.T) {
	s, _ := service(t, open())
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	sh := mustCreate(t, s, author(), share.Request{Report: "summary"})

	raw, _, err := s.Open(context.Background(), sh.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := signer.Verify(raw, token.Portal); err == nil {
		t.Fatal("a share token verified as a portal token")
	}
	claims, err := signer.Verify(raw, token.Embed)
	if err != nil {
		t.Fatalf("a share token did not verify as an embed token: %v", err)
	}
	if claims.Report != "summary" {
		t.Errorf("the token opens %q, want summary", claims.Report)
	}
	// Traceable to the share, not to a person: the recipient is nobody cronos
	// has heard of.
	if claims.Subject != "share:"+sh.ID {
		t.Errorf("subject is %q, want share:%s", claims.Subject, sh.ID)
	}
	// And carrying the id, which is what makes revoking bite before expiry.
	if claims.ID != sh.ID {
		t.Errorf("the token does not carry the share id: %q", claims.ID)
	}
}

/*
A link that never existed, one that expired, and one somebody withdrew all
answer the same way.

Telling them apart hands somebody holding a dead link the fact that a live one
was there — which is a fact about the project that nobody granted them.
*/
func TestEveryWayALinkFailsToOpenLooksTheSame(t *testing.T) {
	s, st := service(t, open())

	revoked := mustCreate(t, s, author(), share.Request{Report: "summary"})
	if err := s.Revoke(context.Background(), author(), revoked.ID); err != nil {
		t.Fatal(err)
	}

	expired := mustCreate(t, s, author(), share.Request{Report: "summary", Days: 1})
	past := time.Now().UTC().AddDate(0, 0, -2)
	sh := st.saved[expired.ID]
	sh.ExpiresAt = &past
	st.saved[expired.ID] = sh

	for _, c := range []struct{ name, id string }{
		{"never existed", "shr_deadbeefdeadbeef"},
		{"withdrawn", revoked.ID},
		{"expired", expired.ID},
	} {
		_, _, err := s.Open(context.Background(), c.id)
		if !errors.Is(err, share.ErrNotOpen) {
			t.Errorf("%s: got %v, want ErrNotOpen", c.name, err)
		}
	}
}

/* -- revoking, and the half a signature cannot do -------------------------- */

func TestAViewerMayNotRevoke(t *testing.T) {
	s, _ := service(t, open())
	sh := mustCreate(t, s, author(), share.Request{Report: "summary"})

	if err := s.Revoke(context.Background(), viewer(), sh.ID); !errors.Is(err, share.ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
}

/*
Allows is asked on every request carrying a share token.

The empty id is the case that must stay true: hosts mint their own embed tokens
for their own users, those carry no share id, and there is nothing here to
revoke. Refusing them would refuse the product's main path.
*/
func TestAllowsRefusesEveryDisagreementBetweenTokenAndRecord(t *testing.T) {
	s, _ := service(t, open())
	live := mustCreate(t, s, author(), share.Request{Report: "summary"})
	gone := mustCreate(t, s, author(), share.Request{Report: "summary"})
	if err := s.Revoke(context.Background(), author(), gone.ID); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name, id, org, project string
		want                   bool
	}{
		{"a host's own token, no share id", "", org, project, true},
		{"live", live.ID, org, project, true},
		{"withdrawn", gone.ID, org, project, false},
		{"never existed", "shr_deadbeefdeadbeef", org, project, false},
		// The cross-tenant cases. A token claiming another organisation than
		// the share was recorded in is the one that must never pass.
		{"another organisation", live.ID, "rival", project, false},
		{"another project", live.ID, org, "hr", false},
	} {
		if got := s.Allows(context.Background(), c.id, c.org, c.project); got != c.want {
			t.Errorf("%s: Allows = %v, want %v", c.name, got, c.want)
		}
	}
}

/* -- listing --------------------------------------------------------------- */

func TestListingShowsOnlyThisProjectsShares(t *testing.T) {
	s, st := service(t, open())
	mine := mustCreate(t, s, author(), share.Request{Report: "summary"})

	// One recorded against somebody else, reachable only through the store.
	theirs := core.Share{ID: "shr_theirs", Org: "rival", Project: "ops", Report: "summary"}
	if err := st.Shared(context.Background(), author(), theirs); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(context.Background(), author())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != mine.ID {
		t.Fatalf("List returned %d shares %v, want only this project's", len(got), got)
	}
}
