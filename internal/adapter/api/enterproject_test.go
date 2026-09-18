package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// orgLike answers with a fixed set of projects, and a fixed set of
// memberships keyed by project.
type orgLike struct {
	projects []string
	member   map[string]string
}

func (o orgLike) ProjectsInOrg(context.Context, string) ([]string, error) {
	return o.projects, nil
}

func (o orgLike) MembershipIn(_ context.Context, _, _, project string) (string, bool, error) {
	role, ok := o.member[project]
	return role, ok, nil
}

func (o orgLike) MembershipsOf(_ context.Context, _ string) ([]identity.Membership, error) {
	out := make([]identity.Membership, 0, len(o.member))
	for project, role := range o.member {
		out = append(out, identity.Membership{Org: "acme", Project: project, Role: role})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Project < out[j].Project })
	return out, nil
}

func enterHandler(t *testing.T) (*api.EnterProject, *token.Signer) {
	t.Helper()
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	h := api.NewEnterProject(orgLike{projects: []string{"finance", "ops"}},
		api.NewAuthor(signer, nil), signer, quiet())
	return h, signer
}

// memberHandler is enterHandler for somebody who belongs to projects rather
// than administering the organization.
func memberHandler(t *testing.T, projects []string, member map[string]string) (*api.EnterProject, *token.Signer) {
	t.Helper()
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return api.NewEnterProject(orgLike{projects: projects, member: member},
		api.NewAuthor(signer, nil), signer, quiet()), signer
}

func enter(t *testing.T, h *api.EnterProject, tok, project string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/project",
		strings.NewReader(`{"project":"`+project+`"}`))
	r.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func mint(t *testing.T, s *token.Signer, c token.Claims) string {
	t.Helper()
	c.Audience = token.Portal
	c.Org, c.Subject = "acme", "usr_ada"
	out, err := s.Mint(c, api.SessionLifetime)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

/*
TestOnlyAnOrgAdminEntersAnotherProject is the check this endpoint exists to
make, and the one it would be easiest to write wrongly.

CanAdminProject is true for an org admin as well as a project admin, because
effective() promotes one into the other. So a handler that checked the
project-level permission — the obvious thing to check on a route about
projects — would let every project administrator in the deployment walk into
every other project of their organization, which is the isolation the whole
tenancy model rests on.
*/
func TestOnlyAnOrgAdminEntersAnotherProject(t *testing.T) {
	h, signer := enterHandler(t)

	for _, c := range []struct {
		name string
		c    token.Claims
	}{
		{"a project administrator", token.Claims{Project: "finance", Role: "admin"}},
		{"an editor", token.Claims{Project: "finance", Role: "editor"}},
		{"a viewer", token.Claims{Project: "finance"}},
		{"an org member, who belongs and administers nothing",
			token.Claims{Project: "finance", OrgRole: "member"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := enter(t, h, mint(t, signer, c.c), "ops")
			/*
			   404 rather than 403, and the difference is deliberate.

			   Since memberships exist there are two reasons this can fail —
			   the project is not there, or they are not in it — and telling
			   them apart lets anybody with a login enumerate an organization's
			   projects one guess at a time. The administrator's path already
			   chose 404 for the same reason, two branches down.
			*/
			if rec.Code != http.StatusNotFound {
				t.Fatalf("got %d, want 404 — it entered a project it may not", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "token") {
				t.Error("and it was handed a session for it")
			}
		})
	}
}

/*
TestAMemberEntersTheirOwnProjectWithItsRole is the other way in.

Somebody in two projects moves between them without administering anything,
which is what the membership table is for. The role comes from the membership
read on this request and never from the session in hand: an editor in finance
who is a viewer in ops arrives in ops as a viewer, and the way that goes wrong
is to carry the role across.
*/
func TestAMemberEntersTheirOwnProjectWithItsRole(t *testing.T) {
	h, signer := memberHandler(t, []string{"finance", "ops"},
		map[string]string{"finance": "editor", "ops": "viewer"})

	rec := enter(t, h, mint(t, signer, token.Claims{Project: "finance", Role: "editor"}), "ops")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 — a member could not enter their own project: %s",
			rec.Code, rec.Body.String())
	}

	var out struct{ Token, Project string }
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Project != "ops" {
		t.Fatalf("landed in %q", out.Project)
	}
	c, err := signer.Verify(out.Token, token.Portal)
	if err != nil {
		t.Fatalf("the session it handed back does not verify: %v", err)
	}
	pr := c.Principal()
	if pr.ProjectRole != principal.ProjectViewer {
		t.Errorf("arrived as %q, and the membership says viewer — the role was "+
			"carried across rather than read", pr.ProjectRole)
	}
	if pr.ProjectID != "ops" {
		t.Errorf("the session names %q", pr.ProjectID)
	}
	if pr.CanAdminProject() {
		t.Error("and it administers the project it only views")
	}
}

// And what they may enter is answerable before they pick, so a switcher offers
// what the endpoint will accept rather than what it will refuse.
func TestTheProjectsOnOfferAreTheOnesThatCanBeEntered(t *testing.T) {
	h, signer := memberHandler(t, []string{"finance", "ops", "legal"},
		map[string]string{"finance": "editor", "ops": "viewer"})

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/project", nil)
	req.Header.Set("Authorization", "Bearer "+
		mint(t, signer, token.Claims{Project: "finance", Role: "editor"}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Projects []struct{ Project, Role, Via string }
		Current  string
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Projects) != 2 {
		t.Fatalf("offered %d projects, want the 2 they are a member of: %+v", len(out.Projects), out.Projects)
	}
	for _, p := range out.Projects {
		if p.Project == "legal" {
			t.Error("offered a project they administer nothing in and belong to not at all")
		}
		if p.Via != "membership" {
			t.Errorf("%s came via %q", p.Project, p.Via)
		}
	}
	if out.Current != "finance" {
		t.Errorf("current is %q", out.Current)
	}
}

// An org owner or admin enters, and what comes back is a session naming the
// project they asked for.
func TestAnOrgAdminEntersAndGetsASessionForThatProject(t *testing.T) {
	h, signer := enterHandler(t)

	for _, role := range []string{"owner", "admin"} {
		t.Run(role, func(t *testing.T) {
			rec := enter(t, h, mint(t, signer,
				token.Claims{Project: "finance", OrgRole: role}), "ops")
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
			}
			var out struct{ Token, Project string }
			if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			if out.Project != "ops" {
				t.Errorf("entered %q, asked for ops", out.Project)
			}

			c, err := signer.Verify(out.Token, token.Portal)
			if err != nil {
				t.Fatalf("the session it minted does not verify: %v", err)
			}
			pr := c.Principal()
			if pr.ProjectID != "ops" {
				t.Errorf("the session names %q", pr.ProjectID)
			}
			// The point of the whole tier: administration of a project they
			// have no membership in.
			if !pr.CanAdminProject() {
				t.Error("and it administers nothing there")
			}
			// Carried over, so revoking the org role ends this session too.
			if !pr.CanAdminOrg() {
				t.Error("the org role did not survive into the new session")
			}
		})
	}
}

/*
TestEnteringIsBoundedByTheOrganization is the containment.

The organization comes from the token and never from the body, so an
administrator of one cannot name a project in another. A project that is not in
their organization is answered as absent rather than refused, because they do
administer this organization and the honest answer is that it has no such
project — a 403 would confirm one exists somewhere else.
*/
func TestEnteringIsBoundedByTheOrganization(t *testing.T) {
	h, signer := enterHandler(t)
	admin := mint(t, signer, token.Claims{Project: "finance", OrgRole: "admin"})

	for _, project := range []string{"someone-elses", "", "../ops"} {
		rec := enter(t, h, admin, project)
		if rec.Code == http.StatusOK {
			t.Errorf("entered %q", project)
		}
	}
}

// An embed token carries no org role at all, so it never reaches the check.
func TestAnEmbedTokenCannotEnterAnything(t *testing.T) {
	h, signer := enterHandler(t)
	forged, err := signer.Mint(token.Claims{
		Audience: token.Embed, Org: "acme", Project: "finance",
		Subject: "cust-1", OrgRole: "owner",
	}, api.SessionLifetime)
	if err != nil {
		t.Fatal(err)
	}
	if rec := enter(t, h, forged, "ops"); rec.Code == http.StatusOK {
		t.Fatal("an end customer of a customer entered another project")
	}
}
