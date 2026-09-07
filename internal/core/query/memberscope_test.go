package query

import (
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Row scope for people who are inside the project.

Membership used to mean "exempt from row scope", and for the person who wrote
the report that is right — an author previewing a scoped dataset with no scope
of their own sees every figure blank. It is wrong for the regional manager who
signs in to read their own region and must not read the others.

The exemption now narrows to the roles that need it. What the tests below pin
is the boundary: an editor sees everything, a viewer with a scope sees their
own rows, and a viewer without one is unchanged from before the feature — which
is what makes upgrading safe.
*/

func member(role principal.Role, scope map[string]string) principal.Principal {
	return principal.Principal{
		Subject: "u1", OrgID: "acme", ProjectID: "finance",
		ProjectRole: role, Member: true, Scope: scope,
	}
}

/*
scoped compiles the row-scoped fixture and says whether the caller was confined.

Confinement is the wrapper, not the words: `customer_id` appears in the
fixture's own JOIN, so looking for the column name reports every caller as
scoped. What distinguishes them is that the dataset query gets wrapped in a
subquery with a predicate against it — no wrapper, no row scope.
*/
func scoped(t *testing.T, pr principal.Principal) (Plan, bool) {
	t.Helper()

	p, err := NewBuilder(SQLite{}).Build(invoices(), map[string]any{
		"from": "2026-01-01", "to": "2026-12-31",
	}, pr)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return p, strings.Contains(p.SQL(), "AS "+subqueryAlias)
}

/*
A viewer an administrator confined reads only their own rows.

This is the feature: somebody inside the project, carrying a scope that was set
for them, gets the dataset's own row-level security applied — the same
predicate an embed token would get, and bound rather than interpolated.
*/
func TestAViewerWithAScopeIsRowScoped(t *testing.T) {
	plan, applied := scoped(t, member(principal.ProjectViewer,
		map[string]string{"customer_id": "c-1"}))

	if !applied {
		t.Fatalf("a confined viewer read unscoped:\n%s", plan.SQL())
	}
	// Bound, never interpolated. A scope that reaches the SQL as text is an
	// injection point wearing a security feature's name.
	if strings.Contains(plan.SQL(), "c-1") {
		t.Errorf("the scope value was interpolated into the SQL:\n%s", plan.SQL())
	}
	found := false
	for _, a := range plan.Args() {
		if a == "c-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("the scope value was not bound as an argument: %v", plan.Args())
	}
}

/*
An editor is still exempt, and that is not an oversight.

Without it the person who wrote the report cannot preview it: they hold no
scope, the predicate matches nothing, and every figure on the page is an em
dash. docs/tenancy.md has said so since before this feature existed.
*/
func TestAnEditorIsStillExemptSoPreviewWorks(t *testing.T) {
	for _, role := range []principal.Role{principal.ProjectEditor, principal.ProjectAdmin} {
		t.Run(string(role), func(t *testing.T) {
			// Even carrying a scope, which an administrator might set on
			// somebody later promoted.
			if _, applied := scoped(t, member(role, map[string]string{"customer_id": "c-1"})); applied {
				t.Errorf("%s was row-scoped and can no longer preview a report", role)
			}
		})
	}
}

// An org administrator holds no project role and resolves to admin, so the
// same exemption reaches them. An org owner who cannot read a broken report is
// how a deployment grows a back door.
func TestAnOrgAdministratorIsExempt(t *testing.T) {
	pr := member("", map[string]string{"customer_id": "c-1"})
	pr.OrgRole = principal.OrgAdmin

	if _, applied := scoped(t, pr); applied {
		t.Error("an org administrator was row-scoped")
	}
}

/*
A viewer with no scope is exactly as it was.

The upgrade safety property. Nobody confined this person, so nothing confines
them, and a deployment that sets no scopes sees no change at all.
*/
func TestAViewerWithNoScopeIsUnchanged(t *testing.T) {
	if _, applied := scoped(t, member(principal.ProjectViewer, nil)); applied {
		t.Error("a viewer nobody scoped was row-scoped anyway")
	}
	if _, applied := scoped(t, member(principal.ProjectViewer, map[string]string{})); applied {
		t.Error("an empty scope was treated as a confinement")
	}
}

// And an end customer is untouched by any of this: no Member at all, so the
// predicate applies as it always has.
func TestAnEndCustomerIsUnaffected(t *testing.T) {
	if _, applied := scoped(t, embedded("c-1")); !applied {
		t.Error("an embed token stopped being row-scoped")
	}
}

/*
The decision itself, at the boundary.

RowScoped is what the builder consults, so it is worth pinning directly rather
than only through compiled SQL — a change here is a change to who sees what.
*/
func TestRowScopedNamesExactlyTheConfinedViewer(t *testing.T) {
	scope := map[string]string{"region": "west"}

	for _, c := range []struct {
		name string
		pr   principal.Principal
		want bool
	}{
		{"viewer with a scope", member(principal.ProjectViewer, scope), true},
		{"viewer with none", member(principal.ProjectViewer, nil), false},
		{"editor with a scope", member(principal.ProjectEditor, scope), false},
		{"admin with a scope", member(principal.ProjectAdmin, scope), false},
	} {
		if got := c.pr.RowScoped(); got != c.want {
			t.Errorf("%s: RowScoped = %v, want %v", c.name, got, c.want)
		}
	}
}
