package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// orgLike answers with a fixed set of projects.
type orgLike struct{ projects []string }

func (o orgLike) ProjectsInOrg(context.Context, string) ([]string, error) {
	return o.projects, nil
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
			if rec.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 403 — it entered a project it may not", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "token") {
				t.Error("and it was handed a session for it")
			}
		})
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
